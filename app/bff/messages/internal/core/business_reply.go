package core

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/business/profile"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

// businessReplyTimeout covers history, contact, presence, the profile write,
// and the shortcut copy. The customer's send does not wait for any of it.
const businessReplyTimeout = 15 * time.Second

// scheduleBusinessAutoReply starts the greeting and away copy after the
// private message is already stored. The RPC returns without waiting on
// MySQL, history, or the shortcut send. A failed copy is logged once.
func (c *MessagesCore) scheduleBusinessAutoReply(peer *mtproto.PeerUtil, sent *mtproto.Updates) {
	if c == nil || c.MD == nil || peer == nil || !peer.IsUser() || peer.PeerId == 0 || peer.PeerId == c.MD.UserId {
		return
	}
	marks := sentMarks(sent)
	ctx, cancel := context.WithTimeout(context.Background(), businessReplyTimeout)
	worker := &MessagesCore{
		ctx:    ctx,
		svcCtx: c.svcCtx,
		Logger: logx.WithContext(ctx),
		MD:     c.MD,
	}
	go func() {
		defer cancel()
		defer func() {
			if rec := recover(); rec != nil {
				worker.Errorf("business auto reply panic: %v", rec)
			}
		}()
		worker.maybeBusinessAutoReply(peer, marks)
	}()
}

// maybeBusinessAutoReply sends the business account's greeting and away
// shortcuts after a private message lands. The customer's own send stays
// successful when the shortcut cannot be copied. Shortcut send errors are
// not retried: the ledger was saved before the ids were returned.
func (c *MessagesCore) maybeBusinessAutoReply(peer *mtproto.PeerUtil, sent sentMark) {
	if c == nil || c.MD == nil || peer == nil || !peer.IsUser() || peer.PeerId == 0 || peer.PeerId == c.MD.UserId {
		return
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MsgClient == nil {
		return
	}
	owner := peer.PeerId
	customer := c.MD.UserId
	card := profile.Shared().Card(owner)
	if card == nil || (card.Greeting == nil && card.Away == nil) {
		return
	}
	now := time.Now()
	facts := profile.PeerFacts{Now: now}
	facts.PriorMessage, facts.HistoryKnown = c.priorPrivateMessage(owner, customer, int32(now.Unix()), sent)
	if c.svcCtx.Dao.UserClient != nil {
		if ok, err := c.svcCtx.Dao.UserClient.UserCheckContact(c.ctx, &userpb.TLUserCheckContact{UserId: owner, Id: customer}); err == nil {
			facts.ContactKnown = true
			facts.Contact = mtproto.FromBool(ok)
		}
		if seen, err := c.svcCtx.Dao.UserClient.UserGetLastSeen(c.ctx, &userpb.TLUserGetLastSeen{Id: owner}); err == nil && seen != nil {
			facts.PresenceKnown = true
			facts.LastSeen = int32(seen.GetLastSeenAt())
			facts.Online = seen.GetExpires() > int32(now.Unix())
		}
	}
	shortcuts := profile.Shared().ConsumeAutoReply(owner, customer, facts)
	for _, shortcutID := range shortcuts {
		if err := c.sendBusinessShortcut(owner, customer, shortcutID); err != nil {
			c.Logger.Errorf("business auto reply owner=%d peer=%d shortcut=%d: %v", owner, customer, shortcutID, err)
		}
	}
}

// priorPrivateMessage is the newest private message before the one this RPC
// just stored. sentIDs are the sender's box ids from that RPC, so the
// sender's own history can ignore them. Any other box is an earlier message.
// The business inbox is only a fallback: its box ids are a different sequence.
type sentMark struct {
	ids     map[int32]struct{}
	randoms map[int64]struct{}
}

func (c *MessagesCore) priorPrivateMessage(owner, customer int64, now int32, sent sentMark) (prior int32, known bool) {
	if prior, known = c.priorExcluding(customer, owner, sent); known {
		return prior, true
	}
	return c.priorByAge(owner, customer, now)
}

func (c *MessagesCore) priorExcluding(userID, peerID int64, sent sentMark) (int32, bool) {
	boxes, ok := c.recentBoxes(userID, peerID)
	if !ok {
		return 0, false
	}
	var prior int32
	other := false
	sawSent := false
	for _, box := range boxes {
		if sent.matches(box) {
			sawSent = true
			continue
		}
		other = true
		if d := messageDate(box); d > prior {
			prior = d
		}
	}
	if other {
		if prior == 0 {
			prior = 1
		}
		return prior, true
	}
	if sawSent {
		return 0, true
	}
	if n, counted := c.messageCount(userID, peerID); counted {
		if n <= 1 {
			return 0, true
		}
		return 1, true
	}
	return 0, false
}

func (c *MessagesCore) priorByAge(userID, peerID int64, now int32) (int32, bool) {
	boxes, ok := c.recentBoxes(userID, peerID)
	if !ok || len(boxes) == 0 {
		if n, counted := c.messageCount(userID, peerID); counted && n > 1 {
			return 1, true
		}
		return 0, false
	}
	var newest, older int32
	for _, box := range boxes {
		d := messageDate(box)
		if d == 0 {
			continue
		}
		if d >= newest {
			older = newest
			newest = d
		} else if d > older {
			older = d
		}
	}
	if newest == 0 {
		return 0, false
	}
	if older > 0 {
		return older, true
	}
	// The customer outbox read already failed. One dated row on the business
	// inbox is an earlier message only when it is older than the copy that
	// just landed. A single current row is not proof of a new chat.
	if now > 0 && now-newest > 120 {
		return newest, true
	}
	return 0, false
}

func (c *MessagesCore) recentBoxes(userID, peerID int64) ([]*mtproto.MessageBox, bool) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, false
	}
	got, err := c.svcCtx.Dao.MessageClient.MessageGetHistoryMessages(c.ctx, &messagepb.TLMessageGetHistoryMessages{
		UserId:   userID,
		PeerType: mtproto.PEER_USER,
		PeerId:   peerID,
		Limit:    20,
	})
	if err != nil || got == nil {
		return nil, false
	}
	return got.GetDatas(), true
}

func (c *MessagesCore) messageCount(userID, peerID int64) (int32, bool) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return 0, false
	}
	got, err := c.svcCtx.Dao.MessageClient.MessageGetHistoryMessagesCount(c.ctx, &messagepb.TLMessageGetHistoryMessagesCount{
		UserId:   userID,
		PeerType: mtproto.PEER_USER,
		PeerId:   peerID,
	})
	if err != nil || got == nil {
		return 0, false
	}
	return got.GetV(), true
}

func sentMarks(ups *mtproto.Updates) sentMark {
	out := sentMark{ids: map[int32]struct{}{}, randoms: map[int64]struct{}{}}
	for _, u := range ups.GetUpdates() {
		name := u.GetPredicateName()
		if name != mtproto.Predicate_updateNewMessage && name != mtproto.Predicate_updateMessageID {
			continue
		}
		id := u.GetId_INT32()
		if id == 0 && u.GetMessage_MESSAGE() != nil {
			id = u.GetMessage_MESSAGE().GetId()
		}
		if id != 0 {
			out.ids[id] = struct{}{}
		}
		if u.GetRandomId() != 0 {
			out.randoms[u.GetRandomId()] = struct{}{}
		}
	}
	return out
}

func (s sentMark) matches(box *mtproto.MessageBox) bool {
	if box == nil {
		return false
	}
	if id := boxID(box); id != 0 {
		if _, ok := s.ids[id]; ok {
			return true
		}
	}
	if box.GetRandomId() != 0 {
		if _, ok := s.randoms[box.GetRandomId()]; ok {
			return true
		}
	}
	return false
}

func boxID(box *mtproto.MessageBox) int32 {
	if box == nil {
		return 0
	}
	if box.GetMessageId() != 0 {
		return box.GetMessageId()
	}
	if box.GetMessage() != nil {
		return box.GetMessage().GetId()
	}
	return 0
}

func messageDate(box *mtproto.MessageBox) int32 {
	if box == nil || box.GetMessage() == nil {
		return 0
	}
	return box.GetMessage().GetDate()
}

func (c *MessagesCore) sendBusinessShortcut(owner, customer int64, shortcutID int32) error {
	page, err := chatbox.Shared().QuickReplyMessages(c.ctx, owner, shortcutID, nil, 0)
	if err != nil {
		return err
	}
	var ids []int32
	var randoms []int64
	for _, m := range page.GetMessages() {
		if m == nil || m.GetId() == 0 {
			continue
		}
		ids = append(ids, m.GetId())
		randoms = append(randoms, freshRandomID())
	}
	if len(ids) == 0 {
		return mtproto.ErrShortcutInvalid
	}
	peer := mtproto.MakeUserPeerUtil(customer)
	prepared, err := chatbox.Shared().PrepareQuickReplySend(c.ctx, owner, peer, shortcutID, ids, randoms)
	if err != nil {
		return err
	}
	boxes := make([]*msg.OutboxMessage, 0, len(prepared))
	now := int32(time.Now().Unix())
	for i, m := range prepared {
		if m == nil {
			continue
		}
		m.Date = now
		boxes = append(boxes, msg.MakeTLOutboxMessage(&msg.OutboxMessage{
			RandomId: randoms[i],
			Message:  m,
		}).To_OutboxMessage())
	}
	if len(boxes) == 0 {
		return mtproto.ErrMessageEmpty
	}
	_, err = c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msg.TLMsgSendMessageV2{
		UserId:    owner,
		AuthKeyId: 0,
		PeerType:  mtproto.PEER_USER,
		PeerId:    customer,
		Message:   boxes,
	})
	return err
}

func freshRandomID() int64 {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	id := int64(binary.LittleEndian.Uint64(buf[:]) & 0x7fffffffffffffff)
	if id == 0 {
		id = 1
	}
	return id
}
