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
	originID := origin.Updates[0].GetMessage_MESSAGE().GetId()
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
	replyID := reply.Updates[0].GetMessage_MESSAGE().GetId()
	if originID == 0 || replyID == 0 || originID == replyID {
		t.Fatalf("ids origin=%d reply=%d", originID, replyID)
	}
	member, err := svc.ReplyToTarget(ctx, 1, in, replyID)
	if err != nil || member != originID {
		t.Fatalf("member target %d %v", member, err)
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

func outsiderCtx(t *testing.T, user int64) context.Context {
	t.Helper()
	out, err := rpcmd.RpcMetadataToOutgoing(context.Background(), &rpcmd.RpcMetadata{UserId: user})
	if err != nil {
		t.Fatal(err)
	}
	md, _ := metadata.FromOutgoingContext(out)
	return metadata.NewIncomingContext(context.Background(), md)
}
