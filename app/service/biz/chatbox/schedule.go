package chatbox

import (
	"context"
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

const (
	maxScheduled     = 100
	maxScheduleAhead = 366 * 24 * time.Hour
)

type schedItem struct {
	ID   int32  `json:"id"`
	When int32  `json:"when"`
	Msg  []byte `json:"msg"`
}

type schedDoc struct {
	Next  int32       `json:"next"`
	Items []schedItem `json:"items"`
}

func schedKey(owner int64, peerType int32, peerID int64) string {
	return "sched:" + userKey(owner) + ":" + peerKey(peerType, peerID)
}

// Schedule stores messages that should be sent later.
//
// https://core.telegram.org/api/scheduled-messages
// The message date is the send time. getScheduledHistory returns them until
// messages.sendScheduledMessages or messages.deleteScheduledMessages.
func (b *Book) Schedule(ctx context.Context, owner int64, peer *mtproto.PeerUtil, when int32, msgs []*mtproto.Message) (*mtproto.Updates, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(msgs) == 0 {
		return nil, mtproto.ErrMessageEmpty
	}
	now := int32(b.now().Unix())
	if when <= now {
		return nil, mtproto.ErrScheduleDateInvalid
	}
	if time.Unix(int64(when), 0).After(b.now().Add(maxScheduleAhead)) {
		return nil, mtproto.ErrScheduleDateTooLate
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := schedKey(owner, peer.PeerType, peer.PeerId)
	doc := b.sched[key]
	if doc == nil {
		doc = &schedDoc{}
	}
	if err := b.load(ctx, key, doc); err != nil {
		return nil, err
	}
	if int(len(doc.Items)+len(msgs)) > maxScheduled {
		return nil, mtproto.ErrScheduleTooMuch
	}
	var ups []*mtproto.Update
	for _, msg := range msgs {
		if msg == nil || (msg.Message == "" && msg.Media == nil) {
			return nil, mtproto.ErrMessageEmpty
		}
		doc.Next++
		cp := proto.Clone(msg).(*mtproto.Message)
		cp.Id = doc.Next
		cp.Out = true
		cp.Date = when
		cp.FromScheduled = false
		cp.FromId = mtproto.MakePeerUser(owner)
		cp.PeerId = peer.ToPeer()
		raw, err := proto.Marshal(cp)
		if err != nil {
			return nil, err
		}
		doc.Items = append(doc.Items, schedItem{ID: cp.Id, When: when, Msg: raw})
		ups = append(ups, mtproto.MakeTLUpdateNewScheduledMessage(&mtproto.Update{
			Message_MESSAGE: cp,
		}).To_Update())
	}
	b.sched[key] = doc
	if err := b.save(ctx, key, doc); err != nil {
		return nil, err
	}
	return mtproto.MakeUpdatesByUpdates(ups...), nil
}

// ScheduledHistory returns the caller's scheduled messages in this dialog.
// A matching hash yields messages.messagesNotModified.
func (b *Book) ScheduledHistory(ctx context.Context, owner int64, peer *mtproto.PeerUtil, hash int64) (*mtproto.Messages_Messages, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	msgs, err := b.listScheduled(ctx, owner, peer)
	if err != nil {
		return nil, err
	}
	sum := hashMessages(msgs)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLMessagesMessagesNotModified(&mtproto.Messages_Messages{
			Count: int32(len(msgs)),
		}).To_Messages_Messages(), nil
	}
	return pageMessages(peer, 0, msgs, nil), nil
}

// ScheduledByID returns the requested scheduled messages. Missing ids are
// messageEmpty, matching messages.getMessages.
func (b *Book) ScheduledByID(ctx context.Context, owner int64, peer *mtproto.PeerUtil, ids []int32) (*mtproto.Messages_Messages, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	have, err := b.listScheduled(ctx, owner, peer)
	if err != nil {
		return nil, err
	}
	byID := map[int32]*mtproto.Message{}
	for _, m := range have {
		byID[m.Id] = m
	}
	out := make([]*mtproto.Message, 0, len(ids))
	for _, id := range ids {
		if m := byID[id]; m != nil {
			out = append(out, m)
			continue
		}
		out = append(out, mtproto.MakeTLMessageEmpty(&mtproto.Message{Id: id, PeerId: peer.ToPeer()}).To_Message())
	}
	return pageMessages(peer, 0, out, nil), nil
}

// TakeScheduled removes messages so they can be sent now. The caller puts
// them back with RestoreScheduled if the send fails.
func (b *Book) TakeScheduled(ctx context.Context, owner int64, peer *mtproto.PeerUtil, ids []int32) ([]*mtproto.Message, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := schedKey(owner, peer.PeerType, peer.PeerId)
	doc, err := b.loadSched(ctx, key)
	if err != nil {
		return nil, err
	}
	want := map[int32]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var taken []*mtproto.Message
	kept := doc.Items[:0]
	for _, item := range doc.Items {
		if !want[item.ID] {
			kept = append(kept, item)
			continue
		}
		msg := &mtproto.Message{}
		if err := proto.Unmarshal(item.Msg, msg); err != nil {
			return nil, err
		}
		taken = append(taken, msg)
		delete(want, item.ID)
	}
	if len(want) > 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	doc.Items = kept
	b.sched[key] = doc
	if err := b.save(ctx, key, doc); err != nil {
		return nil, err
	}
	return taken, nil
}

// RestoreScheduled puts messages back after a failed send.
func (b *Book) RestoreScheduled(ctx context.Context, owner int64, peer *mtproto.PeerUtil, msgs []*mtproto.Message) {
	if peer == nil || len(msgs) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := schedKey(owner, peer.PeerType, peer.PeerId)
	doc, err := b.loadSched(ctx, key)
	if err != nil {
		return
	}
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		raw, err := proto.Marshal(msg)
		if err != nil {
			continue
		}
		doc.Items = append(doc.Items, schedItem{ID: msg.Id, When: msg.Date, Msg: raw})
		if msg.Id > doc.Next {
			doc.Next = msg.Id
		}
	}
	b.sched[key] = doc
	_ = b.save(ctx, key, doc)
}

// DeleteScheduled removes messages and returns updateDeleteScheduledMessages.
func (b *Book) DeleteScheduled(ctx context.Context, owner int64, peer *mtproto.PeerUtil, ids []int32) (*mtproto.Updates, error) {
	msgs, err := b.TakeScheduled(ctx, owner, peer, ids)
	if err != nil {
		return nil, err
	}
	deleted := make([]int32, 0, len(msgs))
	for _, m := range msgs {
		deleted = append(deleted, m.Id)
	}
	up := mtproto.MakeTLUpdateDeleteScheduledMessages(&mtproto.Update{
		Peer_PEER:    peer.ToPeer(),
		Messages:     deleted,
		SentMessages: nil,
	}).To_Update()
	return mtproto.MakeUpdatesByUpdates(up), nil
}

func (b *Book) listScheduled(ctx context.Context, owner int64, peer *mtproto.PeerUtil) ([]*mtproto.Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadSched(ctx, schedKey(owner, peer.PeerType, peer.PeerId))
	if err != nil {
		return nil, err
	}
	out := make([]*mtproto.Message, 0, len(doc.Items))
	for i := len(doc.Items) - 1; i >= 0; i-- {
		msg := &mtproto.Message{}
		if err := proto.Unmarshal(doc.Items[i].Msg, msg); err != nil {
			continue
		}
		out = append(out, msg)
	}
	return out, nil
}

func (b *Book) loadSched(ctx context.Context, key string) (*schedDoc, error) {
	doc := b.sched[key]
	if doc == nil {
		doc = &schedDoc{}
	}
	if err := b.load(ctx, key, doc); err != nil {
		return nil, err
	}
	if doc.Items == nil {
		doc.Items = []schedItem{}
	}
	b.sched[key] = doc
	return doc, nil
}

func hashMessages(msgs []*mtproto.Message) int64 {
	var h int64 = 1
	for _, m := range msgs {
		h = h*20261 + int64(m.GetId())
		h = h*20261 + int64(m.GetDate())
	}
	if h < 0 {
		h = -h
	}
	if h == 0 {
		return 1
	}
	return h
}

// PageMessages builds messages.messages or messages.channelMessages.
func PageMessages(peer *mtproto.PeerUtil, pts int32, msgs []*mtproto.Message, chats []*mtproto.Chat) *mtproto.Messages_Messages {
	return pageMessages(peer, pts, msgs, chats)
}

func pageMessages(peer *mtproto.PeerUtil, pts int32, msgs []*mtproto.Message, chats []*mtproto.Chat) *mtproto.Messages_Messages {
	if msgs == nil {
		msgs = []*mtproto.Message{}
	}
	if chats == nil {
		chats = []*mtproto.Chat{}
	}
	if peer != nil && peer.IsChannel() {
		return mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
			Pts:      pts,
			Count:    int32(len(msgs)),
			Messages: msgs,
			Topics:   []*mtproto.ForumTopic{},
			Chats:    chats,
			Users:    []*mtproto.User{},
		}).To_Messages_Messages()
	}
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: msgs,
		Topics:   []*mtproto.ForumTopic{},
		Chats:    chats,
		Users:    []*mtproto.User{},
	}).To_Messages_Messages()
}

// AffectedHistory is messages.affectedHistory. pts 0 means the dialog pts
// sequence did not move.
func AffectedHistory(pts, count int32) *mtproto.Messages_AffectedHistory {
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      pts,
		PtsCount: count,
		Offset:   0,
	}).To_Messages_AffectedHistory()
}

// DeleteScheduledUpdate is the update clients apply after a scheduled send.
func DeleteScheduledUpdate(peer *mtproto.PeerUtil, scheduled, sent []int32) *mtproto.Update {
	return mtproto.MakeTLUpdateDeleteScheduledMessages(&mtproto.Update{
		Peer_PEER:    peer.ToPeer(),
		Messages:     scheduled,
		SentMessages: sent,
	}).To_Update()
}

func atoi32(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil
}
