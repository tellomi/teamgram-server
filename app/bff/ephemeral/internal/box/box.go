// Package box stores ephemeral messages for one BFF process.
//
// Send and edit return updates whose only entry is updateNewEphemeralMessage
// or updateEditEphemeralMessage. Those constructors have no pts, and the box
// seq is 0, so the client's difference cursor does not move.
package box

import (
	"encoding/binary"
	"hash/fnv"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	maxText      = 4096
	maxEntities  = 100
	maxPerThread = 100
)

// Send is one ephemeral.sendMessage after the handler has checked the session.
type Send struct {
	Owner      int64
	Peer       *mtproto.PeerUtil
	Receiver   int64
	RandomID   int64
	Text       string
	Entities   []*mtproto.MessageEntity
	Markup     *mtproto.ReplyMarkup
	Reply      *mtproto.InputReplyTo
	Welcome    bool
	Invert     bool
	NoForwards bool
	Anchor     bool
	QueryID    *wrapperspb.Int64Value
	Media      *mtproto.InputMedia
	Rich       *mtproto.InputRichMessage
}

type message struct {
	id         int32
	from       int64
	peer       *mtproto.Peer
	receiver   int64
	date       int32
	text       string
	entities   []*mtproto.MessageEntity
	markup     *mtproto.ReplyMarkup
	reply      *mtproto.MessageReplyHeader
	welcome    bool
	invert     bool
	noforwards bool
	anchor     *wrapperspb.Int32Value
	instance   *wrapperspb.Int64Value
	random     int64
}

type thread struct {
	next int32
	msgs []*message
}

type userBox struct {
	threads map[string]*thread
	random  map[int64]int32
}

// Box is the process-local ephemeral store.
type Box struct {
	mu    sync.Mutex
	users map[int64]*userBox
	now   func() time.Time
}

// New returns an empty box.
func New() *Box {
	return &Box{users: map[int64]*userBox{}, now: time.Now}
}

var shared = New()

// Shared is the box the BFF handlers use.
func Shared() *Box {
	return shared
}

// ResetShared drops process state. Tests call it so cases do not leak.
func ResetShared() {
	shared = New()
}

// Send stores a message. The same random_id returns the original updates.
func (b *Box) Send(in Send) (*mtproto.Updates, error) {
	if in.Peer == nil || in.Peer.PeerId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.Receiver == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in.RandomID == 0 {
		return nil, mtproto.ErrRandomIdEmpty
	}
	if err := rejectMedia(in.Media, in.Rich); err != nil {
		return nil, err
	}
	if in.Text == "" {
		return nil, mtproto.ErrMessageEmpty
	}
	if mtproto.UTF16Len(in.Text) > maxText {
		return nil, mtproto.ErrMessageTooLong
	}
	if err := checkEntities(in.Text, in.Entities); err != nil {
		return nil, err
	}
	reply, anchor, err := replyHeader(in.Reply, in.Anchor)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box := b.touch(in.Owner)
	if id, ok := box.random[in.RandomID]; ok {
		if msg := findByID(box, id); msg != nil {
			return newUpdates(msg), nil
		}
	}
	th := box.thread(in.Peer)
	if len(th.msgs) >= maxPerThread {
		return nil, bad("EPHEMERAL_TOO_MUCH")
	}
	th.next++
	msg := &message{
		id:         th.next,
		from:       in.Owner,
		peer:       in.Peer.ToPeer(),
		receiver:   in.Receiver,
		date:       int32(b.now().Unix()),
		text:       in.Text,
		entities:   mtproto.SanitizeMessageEntities(in.Text, in.Entities),
		markup:     cloneMarkup(in.Markup),
		reply:      reply,
		welcome:    in.Welcome,
		invert:     in.Invert,
		noforwards: in.NoForwards,
		anchor:     anchor,
		random:     in.RandomID,
	}
	if in.QueryID != nil {
		msg.instance = wrapperspb.Int64(in.QueryID.GetValue())
	}
	th.msgs = append(th.msgs, msg)
	box.random[in.RandomID] = msg.id
	return newUpdates(msg), nil
}

// Edit replaces text, entities, or markup on one message.
func (b *Box) Edit(owner int64, peer *mtproto.PeerUtil, receiver int64, id int32, text *wrapperspb.StringValue, entities []*mtproto.MessageEntity, markup *mtproto.ReplyMarkup, media *mtproto.InputMedia, rich *mtproto.InputRichMessage, welcome, invert bool) (*mtproto.Updates, error) {
	if peer == nil || peer.PeerId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if receiver == 0 || id <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if err := rejectMedia(media, rich); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	msg := b.message(owner, peer, receiver, id)
	if msg == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	next := msg.text
	if text != nil {
		next = text.GetValue()
		if next == "" {
			return nil, mtproto.ErrMessageEmpty
		}
		if mtproto.UTF16Len(next) > maxText {
			return nil, mtproto.ErrMessageTooLong
		}
	}
	var nextEntities []*mtproto.MessageEntity
	if len(entities) > 0 {
		if err := checkEntities(next, entities); err != nil {
			return nil, err
		}
		nextEntities = mtproto.SanitizeMessageEntities(next, entities)
	} else {
		nextEntities = mtproto.SanitizeMessageEntities(next, msg.entities)
	}
	msg.text = next
	msg.entities = nextEntities
	msg.welcome = welcome
	msg.invert = invert
	msg.date = int32(b.now().Unix())
	if markup != nil {
		msg.markup = cloneMarkup(markup)
	}
	return editUpdates(msg), nil
}

// Delete removes one message addressed to receiver.
func (b *Box) Delete(owner int64, peer *mtproto.PeerUtil, receiver int64, id int32) error {
	if peer == nil || peer.PeerId == 0 {
		return mtproto.ErrPeerIdInvalid
	}
	if receiver == 0 || id <= 0 {
		return mtproto.ErrMessageIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box := b.users[owner]
	if box == nil {
		return mtproto.ErrMessageIdInvalid
	}
	th := box.threads[peerKey(peer)]
	if th == nil || !th.drop(id, receiver, false) {
		return mtproto.ErrMessageIdInvalid
	}
	return nil
}

// DeleteWelcome removes one welcome template in the peer.
func (b *Box) DeleteWelcome(owner int64, peer *mtproto.PeerUtil, id int32) error {
	if peer == nil || peer.PeerId == 0 || id <= 0 {
		return mtproto.ErrMessageIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box := b.users[owner]
	if box == nil {
		return mtproto.ErrMessageIdInvalid
	}
	th := box.threads[peerKey(peer)]
	if th == nil || !th.drop(id, 0, true) {
		return mtproto.ErrMessageIdInvalid
	}
	return nil
}

// DeleteAllWelcome removes every welcome template in the peer.
func (b *Box) DeleteAllWelcome(owner int64, peer *mtproto.PeerUtil) error {
	if peer == nil || peer.PeerId == 0 {
		return mtproto.ErrPeerIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box := b.users[owner]
	if box == nil {
		return nil
	}
	th := box.threads[peerKey(peer)]
	if th == nil {
		return nil
	}
	next := th.msgs[:0]
	for _, msg := range th.msgs {
		if msg.welcome {
			continue
		}
		next = append(next, msg)
	}
	th.msgs = next
	return nil
}

// Welcome returns welcome templates, or the not-modified constructor when hash matches.
func (b *Box) Welcome(owner int64, peer *mtproto.PeerUtil, hash int64) (*mtproto.Ephemeral_WelcomeMessages, error) {
	if peer == nil || peer.PeerId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var msgs []*message
	if box := b.users[owner]; box != nil {
		if th := box.threads[peerKey(peer)]; th != nil {
			for _, msg := range th.msgs {
				if msg.welcome {
					msgs = append(msgs, msg)
				}
			}
		}
	}
	sum := hashMessages(msgs)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLEphemeralWelcomeMessagesNotModified(&mtproto.Ephemeral_WelcomeMessages{}).To_Ephemeral_WelcomeMessages(), nil
	}
	out := make([]*mtproto.EphemeralMessage, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, msg.tl())
	}
	return mtproto.MakeTLEphemeralWelcomeMessages(&mtproto.Ephemeral_WelcomeMessages{
		Hash:     sum,
		Messages: out,
	}).To_Ephemeral_WelcomeMessages(), nil
}

// Callback records a button click and returns an empty bot answer.
// No bot process is invoked.
func (b *Box) Callback(owner int64, peer *mtproto.PeerUtil, id int32, data []byte) (*mtproto.Messages_BotCallbackAnswer, error) {
	if peer == nil || peer.PeerId == 0 || id <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if len(data) == 0 {
		return nil, mtproto.ErrDataInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box := b.users[owner]
	if box == nil || !box.has(peer, id) {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return mtproto.MakeTLMessagesBotCallbackAnswer(&mtproto.Messages_BotCallbackAnswer{
		CacheTime: 0,
	}).To_Messages_BotCallbackAnswer(), nil
}

// Report walks the same option list as messages.report. The message must exist.
func (b *Box) Report(owner int64, peer *mtproto.PeerUtil, id int32, option []byte, comment string) (*mtproto.ReportResult, error) {
	if peer == nil || peer.PeerId == 0 || id <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if len(comment) > 1024 {
		return nil, mtproto.ErrMessageTooLong
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	box := b.users[owner]
	if box == nil || !box.has(peer, id) {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if len(option) == 0 {
		return chooseReport(), nil
	}
	switch string(option) {
	case "spam", "violence", "pornography", "child", "drugs", "personal":
		return reported(), nil
	case "other":
		if comment == "" {
			return mtproto.MakeTLReportResultAddComment(&mtproto.ReportResult{
				Optional: false,
				Option:   []byte("other"),
			}).To_ReportResult(), nil
		}
		return reported(), nil
	default:
		return nil, mtproto.ErrOptionInvalid
	}
}

func (b *Box) touch(user int64) *userBox {
	box := b.users[user]
	if box == nil {
		box = &userBox{threads: map[string]*thread{}, random: map[int64]int32{}}
		b.users[user] = box
	}
	return box
}

func (b *Box) message(owner int64, peer *mtproto.PeerUtil, receiver int64, id int32) *message {
	box := b.users[owner]
	if box == nil {
		return nil
	}
	th := box.threads[peerKey(peer)]
	if th == nil {
		return nil
	}
	for _, msg := range th.msgs {
		if msg.id == id && msg.receiver == receiver {
			return msg
		}
	}
	return nil
}

func (u *userBox) thread(peer *mtproto.PeerUtil) *thread {
	key := peerKey(peer)
	th := u.threads[key]
	if th == nil {
		th = &thread{}
		u.threads[key] = th
	}
	return th
}

func (u *userBox) has(peer *mtproto.PeerUtil, id int32) bool {
	th := u.threads[peerKey(peer)]
	if th == nil {
		return false
	}
	for _, msg := range th.msgs {
		if msg.id == id {
			return true
		}
	}
	return false
}

func (th *thread) drop(id int32, receiver int64, welcomeOnly bool) bool {
	next := th.msgs[:0]
	found := false
	for _, msg := range th.msgs {
		if msg.id == id && (receiver == 0 || msg.receiver == receiver) && (!welcomeOnly || msg.welcome) {
			found = true
			continue
		}
		next = append(next, msg)
	}
	if found {
		th.msgs = next
	}
	return found
}

func findByID(box *userBox, id int32) *message {
	for _, th := range box.threads {
		for _, msg := range th.msgs {
			if msg.id == id {
				return msg
			}
		}
	}
	return nil
}

func (m *message) tl() *mtproto.EphemeralMessage {
	return mtproto.MakeTLEphemeralMessage(&mtproto.EphemeralMessage{
		Out:             true,
		WelcomeTemplate: m.welcome,
		InvertMedia:     m.invert,
		Noforwards:      m.noforwards,
		Id:              m.id,
		FromId:          mtproto.MakePeerUser(m.from),
		PeerId:          proto.Clone(m.peer).(*mtproto.Peer),
		ReceiverId:      m.receiver,
		Date:            m.date,
		Message:         m.text,
		Entities:        cloneEntities(m.entities),
		ReplyMarkup:     cloneMarkup(m.markup),
		ReplyTo:         cloneReply(m.reply),
		ChatInstance:    cloneI64(m.instance),
		AnchorMsgId:     cloneI32(m.anchor),
	}).To_EphemeralMessage()
}

func newUpdates(m *message) *mtproto.Updates {
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNewEphemeralMessage(&mtproto.Update{
		Message_EPHEMERALMESSAGE: m.tl(),
	}).To_Update())
}

func editUpdates(m *message) *mtproto.Updates {
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateEditEphemeralMessage(&mtproto.Update{
		Message_EPHEMERALMESSAGE: m.tl(),
	}).To_Update())
}

func replyHeader(in *mtproto.InputReplyTo, anchor bool) (*mtproto.MessageReplyHeader, *wrapperspb.Int32Value, error) {
	if in == nil {
		if anchor {
			return nil, nil, bad("ANCHOR_INVALID")
		}
		return nil, nil, nil
	}
	var id int32
	ephemeral := false
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputReplyToMessage:
		id = in.GetReplyToMsgId()
	case mtproto.Predicate_inputReplyToEphemeralMessage:
		id = in.GetId()
		ephemeral = true
	default:
		return nil, nil, bad("REPLY_TO_INVALID")
	}
	if id <= 0 {
		return nil, nil, bad("REPLY_TO_INVALID")
	}
	header := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToEphemeral:       ephemeral,
		ReplyToMsgId_FLAGINT32: wrapperspb.Int32(id),
	}).To_MessageReplyHeader()
	var anchorID *wrapperspb.Int32Value
	if anchor {
		anchorID = wrapperspb.Int32(id)
	}
	return header, anchorID, nil
}

func rejectMedia(media *mtproto.InputMedia, rich *mtproto.InputRichMessage) error {
	if media != nil && media.GetPredicateName() != "" && media.GetPredicateName() != mtproto.Predicate_inputMediaEmpty {
		return mtproto.ErrMediaInvalid
	}
	if rich != nil {
		return bad("RICH_MESSAGE_UNSUPPORTED")
	}
	return nil
}

func checkEntities(text string, entities []*mtproto.MessageEntity) error {
	if len(entities) > maxEntities {
		return mtproto.ErrEntitiesTooLong
	}
	n := mtproto.UTF16Len(text)
	for _, e := range entities {
		if e == nil || e.GetLength() <= 0 || e.GetOffset() < 0 || int64(e.GetOffset())+int64(e.GetLength()) > int64(n) {
			return mtproto.ErrEntityBoundsInvalid
		}
	}
	return nil
}

func hashMessages(msgs []*message) int64 {
	h := fnv.New64a()
	for _, msg := range msgs {
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], uint32(msg.id))
		_, _ = h.Write(buf[:])
		_, _ = h.Write([]byte(msg.text))
		_, _ = h.Write([]byte{0})
	}
	v := int64(h.Sum64() & 0x7fffffffffffffff)
	if v == 0 {
		return 1
	}
	return v
}

func peerKey(p *mtproto.PeerUtil) string {
	return itoa(int64(p.PeerType)) + ":" + itoa(p.PeerId)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func chooseReport() *mtproto.ReportResult {
	labels := []struct{ text, id string }{
		{"Spam", "spam"},
		{"Violence", "violence"},
		{"Pornography", "pornography"},
		{"Child abuse", "child"},
		{"Illegal drugs", "drugs"},
		{"Personal details", "personal"},
		{"Other", "other"},
	}
	opts := make([]*mtproto.MessageReportOption, 0, len(labels))
	for _, row := range labels {
		opts = append(opts, mtproto.MakeTLMessageReportOption(&mtproto.MessageReportOption{
			Text:   row.text,
			Option: []byte(row.id),
		}).To_MessageReportOption())
	}
	return mtproto.MakeTLReportResultChooseOption(&mtproto.ReportResult{
		Title:   "What is wrong with this message?",
		Options: opts,
	}).To_ReportResult()
}

func reported() *mtproto.ReportResult {
	return mtproto.MakeTLReportResultReported(&mtproto.ReportResult{}).To_ReportResult()
}

func cloneMarkup(in *mtproto.ReplyMarkup) *mtproto.ReplyMarkup {
	if in == nil {
		return nil
	}
	return proto.Clone(in).(*mtproto.ReplyMarkup)
}

func cloneReply(in *mtproto.MessageReplyHeader) *mtproto.MessageReplyHeader {
	if in == nil {
		return nil
	}
	return proto.Clone(in).(*mtproto.MessageReplyHeader)
}

func cloneEntities(in []*mtproto.MessageEntity) []*mtproto.MessageEntity {
	if len(in) == 0 {
		return nil
	}
	out := make([]*mtproto.MessageEntity, len(in))
	for i, e := range in {
		if e != nil {
			out[i] = proto.Clone(e).(*mtproto.MessageEntity)
		}
	}
	return out
}

func cloneI64(in *wrapperspb.Int64Value) *wrapperspb.Int64Value {
	if in == nil {
		return nil
	}
	return wrapperspb.Int64(in.GetValue())
}

func cloneI32(in *wrapperspb.Int32Value) *wrapperspb.Int32Value {
	if in == nil {
		return nil
	}
	return wrapperspb.Int32(in.GetValue())
}

func bad(code string) error {
	return status.Error(mtproto.ErrBadRequest, code)
}
