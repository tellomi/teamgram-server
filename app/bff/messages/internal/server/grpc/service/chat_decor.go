package service

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

// deliverService posts the service message already built in local so both
// sides of the dialog receive it with a real pts. Without a messenger
// client the local updates are the RPC result.
func deliverService(ctx context.Context, c msg_client.MsgClient, self, auth int64, in *mtproto.InputPeer, local *mtproto.Updates) (*mtproto.Updates, error) {
	if c == nil || local == nil {
		return local, nil
	}
	action := serviceAction(local)
	if action == nil {
		return local, nil
	}
	peer := mtproto.FromInputPeer2(self, in)
	if peer == nil || peer.PeerId == 0 && peer.PeerType != mtproto.PEER_SELF {
		return local, nil
	}
	if peer.PeerType == mtproto.PEER_SELF {
		peer.PeerType = mtproto.PEER_USER
		if peer.PeerId == 0 {
			peer.PeerId = self
		}
	}
	sent, err := c.MsgSendMessageV2(ctx, &msg.TLMsgSendMessageV2{
		UserId:    self,
		AuthKeyId: auth,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		Message: []*msg.OutboxMessage{
			msg.MakeTLOutboxMessage(&msg.OutboxMessage{
				NoWebpage: true,
				RandomId:  randID(),
				Message:   serviceMessage(self, peer, action),
			}).To_OutboxMessage(),
		},
	})
	if err != nil {
		return nil, err
	}
	return sent, nil
}

func serviceAction(up *mtproto.Updates) *mtproto.MessageAction {
	if up == nil {
		return nil
	}
	for _, u := range up.GetUpdates() {
		if m := u.GetMessage_MESSAGE(); m.GetAction() != nil {
			return m.GetAction()
		}
	}
	return nil
}

func serviceMessage(self int64, peer *mtproto.PeerUtil, action *mtproto.MessageAction) *mtproto.Message {
	msg := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		FromId: mtproto.MakePeerUser(self),
		PeerId: peer.ToPeer(),
		Date:   int32(time.Now().Unix()),
		Action: action,
	}).To_Message()
	if peer.IsSelfUser(self) {
		msg.SavedPeerId = mtproto.MakePeerUser(self)
	}
	return msg
}
