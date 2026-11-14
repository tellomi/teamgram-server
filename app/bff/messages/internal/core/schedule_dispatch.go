package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// dispatchOutbox sends now, or stores the messages when schedule_date is set.
//
// https://core.telegram.org/api/scheduled-messages
// A future schedule_date must not hit the messenger. The scheduled inbox
// serves messages.getScheduledHistory until messages.sendScheduledMessages.
func (c *MessagesCore) dispatchOutbox(peer *mtproto.PeerUtil, boxes []*msg.OutboxMessage) (*mtproto.Updates, error) {
	var when int32
	repeating := false
	for _, box := range boxes {
		if v := box.GetScheduleDate().GetValue(); v != 0 {
			when = v
		}
		if box.GetMessage().GetScheduleRepeatPeriod().GetValue() != 0 {
			repeating = true
		}
	}
	// A date less than 10 seconds ahead is a normal send. The client does
	// not get updateNewScheduledMessage for that window.
	if chatbox.SendInsteadOfSchedule(when, int32(time.Now().Unix()), repeating) {
		for _, box := range boxes {
			box.ScheduleDate = nil
		}
		when = 0
	}
	if when != 0 {
		if when == chatbox.ScheduleWhenOnline && !c.allowScheduleOnline(peer) {
			return nil, mtproto.ErrScheduleStatusPrivate
		}
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

// stampScheduleRepeat copies schedule_repeat_period onto the message that
// chatbox.Schedule stores. A period without a future schedule_date, a period
// combined with send-when-online, or a period clients do not send is
// SCHEDULE_DATE_INVALID.
func stampScheduleRepeat(msg *mtproto.Message, period *wrapperspb.Int32Value, when int32) error {
	if msg == nil || period == nil || period.GetValue() == 0 {
		return nil
	}
	if when == 0 || when == chatbox.ScheduleWhenOnline || !chatbox.ValidRepeatPeriod(period.GetValue()) {
		return mtproto.ErrScheduleDateInvalid
	}
	msg.ScheduleRepeatPeriod = period
	return nil
}

// allowScheduleOnline is false when the peer hides last seen from the sender.
// A failed lookup does not block the schedule; the method page's
// SCHEDULE_STATUS_PRIVATE is for a privacy refusal we could actually read.
func (c *MessagesCore) allowScheduleOnline(peer *mtproto.PeerUtil) bool {
	if peer == nil || peer.PeerType != mtproto.PEER_USER || peer.PeerId == 0 || peer.PeerId == c.MD.UserId {
		return false
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return true
	}
	rules, err := c.svcCtx.Dao.UserClient.UserGetPrivacy(c.ctx, &userpb.TLUserGetPrivacy{
		UserId:  peer.PeerId,
		KeyType: mtproto.STATUS_TIMESTAMP,
	})
	if err != nil || rules == nil || len(rules.Datas) == 0 {
		return true
	}
	return mtproto.CheckPrivacyIsAllow(
		peer.PeerId,
		rules.Datas,
		c.MD.UserId,
		func(id, checkID int64) bool {
			contact, _ := c.svcCtx.Dao.UserClient.UserCheckContact(c.ctx, &userpb.TLUserCheckContact{
				UserId: id,
				Id:     checkID,
			})
			return mtproto.FromBool(contact)
		},
		func(checkID int64, idList []int64) bool {
			if c.svcCtx.Dao.ChatClient == nil {
				return false
			}
			chatIDList, _ := mtproto.SplitChatAndChannelIdList(idList)
			return c.svcCtx.Dao.ChatClient.CheckParticipantIsExist(c.ctx, checkID, chatIDList)
		})
}
