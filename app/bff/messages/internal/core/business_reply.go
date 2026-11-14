package core

import (
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/business/profile"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// maybeBusinessAutoReply sends the business account's greeting and away
// shortcuts after a private message lands. The customer's own send stays
// successful when the shortcut cannot be copied.
func (c *MessagesCore) maybeBusinessAutoReply(peer *mtproto.PeerUtil) {
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
	if c.svcCtx.Dao.UserClient != nil {
		if ok, err := c.svcCtx.Dao.UserClient.UserCheckContact(c.ctx, &userpb.TLUserCheckContact{UserId: owner, Id: customer}); err == nil {
			facts.Contact = mtproto.FromBool(ok)
		}
		if seen, err := c.svcCtx.Dao.UserClient.UserGetLastSeen(c.ctx, &userpb.TLUserGetLastSeen{Id: owner}); err == nil && seen != nil {
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
