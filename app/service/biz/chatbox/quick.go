package chatbox

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Quick replies are per-user shortcut inboxes.
// https://core.telegram.org/api/business#quick-reply-shortcuts
//
// messages.sendMessage, sendMedia, and sendMultiMedia add a message when
// quick_reply_shortcut is set and the peer is the current user.
// messages.sendQuickReplyMessages copies those messages into another chat.

const (
	maxQuickReplies        = 100
	maxQuickReplyMessages  = 20
	maxQuickReplyNameRunes = 32
)

type quickDoc struct {
	NextID    int32           `json:"next_id"`
	NextMsg   int32           `json:"next_msg"`
	Shortcuts []quickShortcut `json:"shortcuts,omitempty"`
}

type quickShortcut struct {
	ID       int32      `json:"id"`
	Name     string     `json:"name"`
	Messages []quickMsg `json:"messages,omitempty"`
}

type quickMsg struct {
	ID  int32  `json:"id"`
	Raw []byte `json:"raw"`
}

func quickKey(user int64) string { return "quick:" + userKey(user) }

// CheckQuickReplyShortcut reports whether shortcut is a free, valid name.
// https://core.telegram.org/method/messages.checkQuickReplyShortcut
func (b *Book) CheckQuickReplyShortcut(ctx context.Context, user int64, name string) (*mtproto.Bool, error) {
	norm, ok := normalizeShortcut(name)
	if !ok {
		return nil, mtproto.ErrShortcutInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	if findShortcut(doc, norm) >= 0 {
		return mtproto.BoolFalse, nil
	}
	return mtproto.BoolTrue, nil
}

// QuickReplies is messages.getQuickReplies.
// A matching hash returns messages.quickRepliesNotModified.
func (b *Book) QuickReplies(ctx context.Context, user int64, hash int64) (*mtproto.Messages_QuickReplies, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	replies, tops := shortcutView(doc)
	sum := hashQuick(replies, tops)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesQuickRepliesNotModified(&mtproto.Messages_QuickReplies{}).To_Messages_QuickReplies(), nil
	}
	return mtproto.MakeTLMessagesQuickReplies(&mtproto.Messages_QuickReplies{
		QuickReplies: replies,
		Messages:     tops,
		Chats:        []*mtproto.Chat{},
		Users:        []*mtproto.User{},
	}).To_Messages_QuickReplies(), nil
}

// QuickReplyMessages is messages.getQuickReplyMessages.
func (b *Book) QuickReplyMessages(ctx context.Context, user int64, shortcutID int32, ids []int32, hash int64) (*mtproto.Messages_Messages, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	sc := shortcutByID(doc, shortcutID)
	if sc == nil {
		return nil, mtproto.ErrShortcutInvalid
	}
	msgs, err := filterQuick(sc, ids)
	if err != nil {
		return nil, err
	}
	sum := hashMessages(msgs)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesMessagesNotModified(&mtproto.Messages_Messages{
			Count: int32(len(msgs)),
		}).To_Messages_Messages(), nil
	}
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: msgs,
		Topics:   []*mtproto.ForumTopic{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

// EditQuickReplyShortcut renames a shortcut.
// https://core.telegram.org/method/messages.editQuickReplyShortcut
func (b *Book) EditQuickReplyShortcut(ctx context.Context, user int64, shortcutID int32, name string) (*mtproto.Bool, error) {
	norm, ok := normalizeShortcut(name)
	if !ok {
		return nil, mtproto.ErrShortcutInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	idx := indexShortcut(doc, shortcutID)
	if idx < 0 {
		return nil, mtproto.ErrShortcutInvalid
	}
	if other := findShortcut(doc, norm); other >= 0 && other != idx {
		return nil, mtproto.ErrShortcutInvalid
	}
	doc.Shortcuts[idx].Name = norm
	if err := b.storeQuick(ctx, user, doc); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// DeleteQuickReplyShortcut removes a shortcut and its messages.
func (b *Book) DeleteQuickReplyShortcut(ctx context.Context, user int64, shortcutID int32) (*mtproto.Bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	idx := indexShortcut(doc, shortcutID)
	if idx < 0 {
		return nil, mtproto.ErrShortcutInvalid
	}
	doc.Shortcuts = append(doc.Shortcuts[:idx], doc.Shortcuts[idx+1:]...)
	if err := b.storeQuick(ctx, user, doc); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// ReorderQuickReplies applies order as a permutation of every shortcut id.
func (b *Book) ReorderQuickReplies(ctx context.Context, user int64, order []int32) (*mtproto.Bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	if len(order) != len(doc.Shortcuts) {
		return nil, mtproto.ErrShortcutInvalid
	}
	byID := map[int32]quickShortcut{}
	for _, sc := range doc.Shortcuts {
		byID[sc.ID] = sc
	}
	next := make([]quickShortcut, 0, len(order))
	seen := map[int32]bool{}
	for _, id := range order {
		sc, ok := byID[id]
		if !ok || seen[id] {
			return nil, mtproto.ErrShortcutInvalid
		}
		seen[id] = true
		next = append(next, sc)
	}
	doc.Shortcuts = next
	if err := b.storeQuick(ctx, user, doc); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// DeleteQuickReplyMessages drops messages from a shortcut.
// https://core.telegram.org/method/messages.deleteQuickReplyMessages
func (b *Book) DeleteQuickReplyMessages(ctx context.Context, user int64, shortcutID int32, ids []int32) (*mtproto.Updates, error) {
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	sc := shortcutByID(doc, shortcutID)
	if sc == nil {
		return nil, mtproto.ErrShortcutInvalid
	}
	drop := map[int32]bool{}
	for _, id := range ids {
		if id <= 0 || !quickHas(sc, id) {
			return nil, mtproto.ErrMessageIdInvalid
		}
		drop[id] = true
	}
	kept := sc.Messages[:0]
	for _, m := range sc.Messages {
		if !drop[m.ID] {
			kept = append(kept, m)
		}
	}
	sc.Messages = kept
	if err := b.storeQuick(ctx, user, doc); err != nil {
		return nil, err
	}
	card := shortcutCard(sc)
	return mtproto.MakeUpdatesByUpdates(
		mtproto.MakeTLUpdateNewQuickReply(&mtproto.Update{QuickReply: card}).To_Update(),
		mtproto.MakeTLUpdateDeleteQuickReplyMessages(&mtproto.Update{
			ShortcutId: shortcutID,
			Messages:   append([]int32(nil), ids...),
		}).To_Update(),
	), nil
}

// PrepareQuickReplySend copies shortcut messages for messages.sendQuickReplyMessages.
// The caller delivers them through messenger.msg.
func (b *Book) PrepareQuickReplySend(ctx context.Context, user int64, peer *mtproto.PeerUtil, shortcutID int32, ids []int32, random []int64) ([]*mtproto.Message, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	if len(random) != len(ids) {
		return nil, mtproto.ErrRandomIdInvalid
	}
	seenRand := map[int64]bool{}
	for _, id := range random {
		if id == 0 || seenRand[id] {
			return nil, mtproto.ErrRandomIdInvalid
		}
		seenRand[id] = true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	sc := shortcutByID(doc, shortcutID)
	if sc == nil {
		return nil, mtproto.ErrShortcutInvalid
	}
	out := make([]*mtproto.Message, 0, len(ids))
	for _, id := range ids {
		raw := quickRaw(sc, id)
		if raw == nil {
			return nil, mtproto.ErrMessageIdInvalid
		}
		msg := &mtproto.Message{}
		if err := proto.Unmarshal(raw, msg); err != nil {
			return nil, err
		}
		msg.Id = 0
		msg.QuickReplyShortcutId = nil
		msg.Out = true
		msg.PeerId = peer.ToPeer()
		msg.FromId = mtproto.MakePeerUser(user)
		out = append(out, msg)
	}
	return out, nil
}

// CaptureQuickReply stores messages into a shortcut instead of a chat.
// peer must be the current user, matching the quick-reply send rules.
func (b *Book) CaptureQuickReply(ctx context.Context, user int64, peer *mtproto.PeerUtil, shortcut *mtproto.InputQuickReplyShortcut, msgs []*mtproto.Message) (*mtproto.Updates, error) {
	if shortcut == nil {
		return nil, mtproto.ErrShortcutInvalid
	}
	if peer == nil || !peer.IsSelfUser(user) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(msgs) == 0 {
		return nil, mtproto.ErrMessageEmpty
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadQuick(ctx, user)
	if err != nil {
		return nil, err
	}
	idx, err := resolveShortcut(doc, shortcut)
	if err != nil {
		return nil, err
	}
	var sc *quickShortcut
	if idx < 0 {
		if len(doc.Shortcuts) >= maxQuickReplies {
			return nil, mtproto.ErrQuickRepliesTooMuch
		}
		norm, ok := normalizeShortcut(shortcut.GetShortcut())
		if !ok {
			return nil, mtproto.ErrShortcutInvalid
		}
		doc.NextID++
		doc.Shortcuts = append(doc.Shortcuts, quickShortcut{ID: doc.NextID, Name: norm})
		idx = len(doc.Shortcuts) - 1
	}
	sc = &doc.Shortcuts[idx]
	if len(sc.Messages)+len(msgs) > maxQuickReplyMessages {
		return nil, mtproto.ErrQuickRepliesTooMuch
	}
	var updates []*mtproto.Update
	now := int32(b.now().Unix())
	for _, src := range msgs {
		if src == nil {
			return nil, mtproto.ErrMessageEmpty
		}
		if src.GetMessage() == "" && src.GetMedia() == nil {
			return nil, mtproto.ErrMessageEmpty
		}
		doc.NextMsg++
		msg := proto.Clone(src).(*mtproto.Message)
		msg.Id = doc.NextMsg
		msg.Date = now
		msg.Out = true
		msg.QuickReplyShortcutId = wrapperspb.Int32(sc.ID)
		raw, err := proto.Marshal(msg)
		if err != nil {
			return nil, err
		}
		sc.Messages = append(sc.Messages, quickMsg{ID: msg.Id, Raw: raw})
		updates = append(updates, mtproto.MakeTLUpdateQuickReplyMessage(&mtproto.Update{
			Message_MESSAGE: msg,
		}).To_Update())
	}
	if err := b.storeQuick(ctx, user, doc); err != nil {
		return nil, err
	}
	card := shortcutCard(sc)
	updates = append([]*mtproto.Update{
		mtproto.MakeTLUpdateNewQuickReply(&mtproto.Update{QuickReply: card}).To_Update(),
	}, updates...)
	return mtproto.MakeUpdatesByUpdates(updates...), nil
}

func (b *Book) loadQuick(ctx context.Context, user int64) (*quickDoc, error) {
	doc := b.quick[user]
	if doc == nil {
		doc = &quickDoc{}
	}
	if err := b.load(ctx, quickKey(user), doc); err != nil {
		return nil, err
	}
	b.quick[user] = doc
	return doc, nil
}

func (b *Book) storeQuick(ctx context.Context, user int64, doc *quickDoc) error {
	b.quick[user] = doc
	return b.save(ctx, quickKey(user), doc)
}

func resolveShortcut(doc *quickDoc, in *mtproto.InputQuickReplyShortcut) (int, error) {
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputQuickReplyShortcutId:
		idx := indexShortcut(doc, in.GetShortcutId())
		if idx < 0 {
			return -1, mtproto.ErrShortcutInvalid
		}
		return idx, nil
	case mtproto.Predicate_inputQuickReplyShortcut, "":
		norm, ok := normalizeShortcut(in.GetShortcut())
		if !ok {
			return -1, mtproto.ErrShortcutInvalid
		}
		return findShortcut(doc, norm), nil
	default:
		return -1, mtproto.ErrShortcutInvalid
	}
}

func normalizeShortcut(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxQuickReplyNameRunes {
		return "", false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return "", false
	}
	return strings.ToLower(name), true
}

func findShortcut(doc *quickDoc, norm string) int {
	for i, sc := range doc.Shortcuts {
		if strings.EqualFold(sc.Name, norm) {
			return i
		}
	}
	return -1
}

func indexShortcut(doc *quickDoc, id int32) int {
	for i, sc := range doc.Shortcuts {
		if sc.ID == id {
			return i
		}
	}
	return -1
}

func shortcutByID(doc *quickDoc, id int32) *quickShortcut {
	idx := indexShortcut(doc, id)
	if idx < 0 {
		return nil
	}
	return &doc.Shortcuts[idx]
}

func shortcutView(doc *quickDoc) ([]*mtproto.QuickReply, []*mtproto.Message) {
	replies := make([]*mtproto.QuickReply, 0, len(doc.Shortcuts))
	var tops []*mtproto.Message
	for i := range doc.Shortcuts {
		sc := &doc.Shortcuts[i]
		replies = append(replies, shortcutCard(sc))
		if top := topQuick(sc); top != nil {
			tops = append(tops, top)
		}
	}
	if tops == nil {
		tops = []*mtproto.Message{}
	}
	return replies, tops
}

func shortcutCard(sc *quickShortcut) *mtproto.QuickReply {
	topID := int32(0)
	if top := topQuick(sc); top != nil {
		topID = top.GetId()
	}
	return mtproto.MakeTLQuickReply(&mtproto.QuickReply{
		ShortcutId: sc.ID,
		Shortcut:   sc.Name,
		TopMessage: topID,
		Count:      int32(len(sc.Messages)),
	}).To_QuickReply()
}

func topQuick(sc *quickShortcut) *mtproto.Message {
	if len(sc.Messages) == 0 {
		return nil
	}
	return decodeQuick(sc.Messages[len(sc.Messages)-1].Raw)
}

func filterQuick(sc *quickShortcut, ids []int32) ([]*mtproto.Message, error) {
	if len(ids) == 0 {
		out := make([]*mtproto.Message, 0, len(sc.Messages))
		for _, m := range sc.Messages {
			if msg := decodeQuick(m.Raw); msg != nil {
				out = append(out, msg)
			}
		}
		return out, nil
	}
	out := make([]*mtproto.Message, 0, len(ids))
	for _, id := range ids {
		raw := quickRaw(sc, id)
		if raw == nil {
			return nil, mtproto.ErrMessageIdInvalid
		}
		msg := decodeQuick(raw)
		if msg == nil {
			return nil, mtproto.ErrMessageIdInvalid
		}
		out = append(out, msg)
	}
	return out, nil
}

func quickHas(sc *quickShortcut, id int32) bool { return quickRaw(sc, id) != nil }

func quickRaw(sc *quickShortcut, id int32) []byte {
	for _, m := range sc.Messages {
		if m.ID == id {
			return m.Raw
		}
	}
	return nil
}

func decodeQuick(raw []byte) *mtproto.Message {
	if len(raw) == 0 {
		return nil
	}
	msg := &mtproto.Message{}
	if err := proto.Unmarshal(raw, msg); err != nil {
		return nil
	}
	return msg
}

func hashQuick(replies []*mtproto.QuickReply, tops []*mtproto.Message) int64 {
	var h int64 = 1
	for _, r := range replies {
		h = h*20261 + int64(r.GetShortcutId())
		h = h*20261 + int64(r.GetTopMessage())
		h = h*20261 + int64(r.GetCount())
	}
	for _, m := range tops {
		h = h*20261 + int64(m.GetId())
	}
	if h < 0 {
		h = -h
	}
	if h == 0 {
		return 1
	}
	return h
}
