package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// privateInboxDelivered is false when the recipient has blocked the sender.
// The sender's outbox is still written, so their client shows the message as
// sent. The blocker does not get an inbox copy. This is the official private
// chat behavior and is already what the send and edit handlers do.
func privateInboxDelivered(recipientBlockedSender bool) bool {
	return !recipientBlockedSender
}

// privateInboxReqs is the sender copy plus, when deliverRecipient is set, the
// other party's copy. The same list is built for the first publish and for a
// retry after that publish failed.
func privateInboxReqs(fromID, toID, authKeyID int64, boxes []*mtproto.MessageBox, senderUsers, recipientUsers []*mtproto.User, deliverRecipient bool) []*inbox.TLInboxSendUserMessageToInboxV2 {
	reqs := []*inbox.TLInboxSendUserMessageToInboxV2{{
		UserId:        fromID,
		Out:           true,
		FromId:        fromID,
		FromAuthKeyId: authKeyID,
		PeerType:      mtproto.PEER_USER,
		PeerId:        toID,
		BoxList:       boxes,
		Users:         senderUsers,
	}}
	if fromID != toID && deliverRecipient {
		reqs = append(reqs, &inbox.TLInboxSendUserMessageToInboxV2{
			UserId:        toID,
			Out:           false,
			FromId:        fromID,
			FromAuthKeyId: authKeyID,
			PeerType:      mtproto.PEER_USER,
			PeerId:        toID,
			BoxList:       boxes,
			Users:         recipientUsers,
		})
	}
	return reqs
}

func (c *MsgCore) privateUserInbox(ctx context.Context, fromID, authKeyID, toID int64, boxes []*mtproto.MessageBox, users *userpb.Vector_ImmutableUser, idHelper *mtproto.IDListHelper) []*inbox.TLInboxSendUserMessageToInboxV2 {
	deliver := false
	if fromID != toID && c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.UserClient != nil {
		blocked, _ := c.svcCtx.Dao.UserClient.UserBlockedByUser(ctx, &userpb.TLUserBlockedByUser{
			UserId:     toID,
			PeerUserId: fromID,
		})
		deliver = privateInboxDelivered(mtproto.FromBool(blocked))
	}
	var senderUsers, recipientUsers []*mtproto.User
	if users != nil && idHelper != nil {
		senderUsers = users.GetUserListByIdList(fromID, idHelper.UserIdList...)
		recipientUsers = users.GetUserListByIdList(toID, idHelper.UserIdList...)
	}
	return privateInboxReqs(fromID, toID, authKeyID, boxes, senderUsers, recipientUsers, deliver)
}
