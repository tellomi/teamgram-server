package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

// dispatchOutbox sends now, or stores the messages when schedule_date is set.
//
// https://core.telegram.org/api/scheduled-messages
// A future schedule_date must not hit the messenger. The scheduled inbox
// serves messages.getScheduledHistory until messages.sendScheduledMessages.
func (c *MessagesCore) dispatchOutbox(peer *mtproto.PeerUtil, boxes []*msg.OutboxMessage) (*mtproto.Updates, error) {
	var when int32
	for _, box := range boxes {
		if v := box.GetScheduleDate().GetValue(); v != 0 {
			when = v
		}
	}
	if when != 0 {
		if peer.IsChannel() {
			if _, _, err := channel.Shared().Card(c.ctx, c.MD.UserId, peer.PeerId); err != nil {
				return nil, err
			}
		}
		msgs := make([]*mtproto.Message, 0, len(boxes))
		for _, box := range boxes {
			if box.GetMessage() == nil {
				return nil, mtproto.ErrMessageEmpty
			}
			msgs = append(msgs, box.GetMessage())
		}
		ups, err := chatbox.Shared().Schedule(c.ctx, c.MD.UserId, peer, when, msgs)
		if err != nil {
			return nil, err
		}
		if peer.IsChannel() {
			if _, chat, err := channel.Shared().Card(c.ctx, c.MD.UserId, peer.PeerId); err == nil && chat != nil {
				ups.Chats = []*mtproto.Chat{chat}
			}
		}
		return ups, nil
	}
	ups, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msg.TLMsgSendMessageV2{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		Message:   boxes,
	})
	if err != nil {
		return nil, err
	}
	chatbox.Shared().ObserveSent(c.ctx, peer, ups)
	return ups, nil
}
