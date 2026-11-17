package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestGetMessagesReplyToOutsiderGetsTheSameError(t *testing.T) {
	svc := channel.New(channel.NewMemoryStore())
	channel.Use(svc)
	t.Cleanup(func() { channel.Use(channel.New(channel.NewMemoryStore())) })
	ctx := context.Background()
	created, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Quiet", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  ch.GetId(),
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
	origin, err := svc.Send(ctx, 1, ch.GetId(), 0, []channel.Outgoing{{
		RandomID: 1,
		Message:  &mtproto.Message{Message: "origin", Date: 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	originID := sentChannelPostID(t, origin)
	reply, err := svc.Send(ctx, 1, ch.GetId(), 0, []channel.Outgoing{{
		RandomID: 2,
		Message: &mtproto.Message{
			Message: "reply",
			Date:    11,
			ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
				ReplyToMsgId_FLAGINT32: &wrapperspb.Int32Value{Value: originID},
			}).To_MessageReplyHeader(),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	replyID := sentChannelPostID(t, reply)
	plain, err := svc.Send(ctx, 1, ch.GetId(), 0, []channel.Outgoing{{
		RandomID: 3,
		Message: &mtproto.Message{
			Message: "plain",
			Date:    12,
			ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
				ReplyToMsgId: originID,
			}).To_MessageReplyHeader(),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	plainID := sentChannelPostID(t, plain)
	if originID == 0 || replyID == 0 || plainID == 0 || originID == replyID || originID == plainID {
		t.Fatalf("ids origin=%d flag=%d plain=%d", originID, replyID, plainID)
	}
	member, err := svc.ReplyToTarget(ctx, 1, in, replyID)
	if err != nil || member != originID {
		t.Fatalf("flag target %d %v", member, err)
	}
	member, err = svc.ReplyToTarget(ctx, 1, in, plainID)
	if err != nil || member != originID {
		stored, _ := svc.LoadMessage(ctx, ch.GetId(), plainID)
		t.Fatalf("plain target %d %v header=%+v", member, err, stored.GetReplyTo())
	}

	outsider := outsiderCtx(t, 99)
	handler := &Service{}
	missing, errMissing := handler.ChannelsGetMessages(outsider, replyToRequest(in, 99999))
	stored, errStored := handler.ChannelsGetMessages(outsider, replyToRequest(in, replyID))
	if missing != nil || stored != nil {
		t.Fatalf("body missing=%#v stored=%#v", missing, stored)
	}
	if errMissing == nil || errStored == nil || errMissing != errStored || errMissing != mtproto.ErrChannelPrivate {
		t.Fatalf("missing %v stored %v", errMissing, errStored)
	}
}

func replyToRequest(in *mtproto.InputChannel, id int32) *mtproto.TLChannelsGetMessages {
	return &mtproto.TLChannelsGetMessages{
		Channel: in,
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{
			mtproto.MakeTLInputMessageReplyTo(&mtproto.InputMessage{Id: id}).To_InputMessage(),
		},
	}
}

func sentChannelPostID(t *testing.T, up *mtproto.Updates) int32 {
	t.Helper()
	for _, u := range up.GetUpdates() {
		if u.GetPredicateName() == mtproto.Predicate_updateNewChannelMessage && u.GetMessage_MESSAGE().GetId() != 0 {
			return u.GetMessage_MESSAGE().GetId()
		}
	}
	t.Fatal("send result has no updateNewChannelMessage")
	return 0
}

func outsiderCtx(t *testing.T, user int64) context.Context {
	t.Helper()
	out, err := rpcmd.RpcMetadataToOutgoing(context.Background(), &rpcmd.RpcMetadata{UserId: user})
	if err != nil {
		t.Fatal(err)
	}
	md, _ := metadata.FromOutgoingContext(out)
	return metadata.NewIncomingContext(context.Background(), md)
}
