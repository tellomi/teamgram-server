package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestToggleNoForwardsRejectsForward(t *testing.T) {
	prev := channel.Shared()
	svc := channel.New(channel.NewMemoryStore())
	channel.Use(svc)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	created, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Locked", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: ch.Id, AccessHash: ch.GetAccessHash_FLAGINT64().GetValue()}
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "lockedroom"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Join(ctx, 2, 0, in); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []channel.Outgoing{{RandomID: 4, Message: &mtproto.Message{Message: "keep", Date: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].Message_MESSAGE.Id
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId:  ch.Id,
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputPeer()

	member := &MessagesCore{ctx: ctx, Logger: logx.WithContext(ctx), MD: &metadata.RpcMetadata{UserId: 2}}
	if _, err := member.MessagesToggleNoForwards(&mtproto.TLMessagesToggleNoForwards{Peer: peer, Enabled: mtproto.BoolTrue}); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member toggle: %v", err)
	}
	userPeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer()
	owner := &MessagesCore{ctx: ctx, Logger: logx.WithContext(ctx), MD: &metadata.RpcMetadata{UserId: 1, PermAuthKeyId: 8}}
	if _, err := owner.MessagesToggleNoForwards(&mtproto.TLMessagesToggleNoForwards{Peer: userPeer, Enabled: mtproto.BoolTrue}); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatalf("user peer: %v", err)
	}
	up, err := owner.MessagesToggleNoForwards(&mtproto.TLMessagesToggleNoForwards{Peer: peer, Enabled: mtproto.BoolTrue})
	if err != nil {
		t.Fatal(err)
	}
	if len(up.GetChats()) == 0 || !up.GetChats()[0].GetNoforwards() {
		t.Fatalf("enabled chat %#v", up.GetChats())
	}
	from := mtproto.FromInputPeer2(1, peer)
	to := mtproto.FromInputPeer2(1, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer())
	if _, err := owner.makeForwardMessages(from, to, false, &mtproto.TLMessagesForwardMessages{
		Id: []int32{msgID}, RandomId: []int64{9},
	}); status.Convert(err).Message() != "CHAT_FORWARDS_RESTRICTED" {
		t.Fatalf("forward: %v", err)
	}
	if _, err := owner.MessagesToggleNoForwards(&mtproto.TLMessagesToggleNoForwards{Peer: peer, Enabled: mtproto.BoolFalse}); err != nil {
		t.Fatal(err)
	}
	boxes, err := owner.makeForwardMessages(from, to, false, &mtproto.TLMessagesForwardMessages{
		Id: []int32{msgID}, RandomId: []int64{10},
	})
	if err != nil || len(boxes) != 1 {
		t.Fatalf("forward after clear: %v %#v", err, boxes)
	}
}
