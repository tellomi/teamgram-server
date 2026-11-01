package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestGetMessagesViewsChannelAndForward(t *testing.T) {
	prev := channel.Shared()
	chSvc := channel.New(channel.NewMemoryStore())
	channel.Use(chSvc)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	up, err := chSvc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  ch.Id,
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
	if _, err := chSvc.UpdateUsername(ctx, 1, 0, in, "news_views"); err != nil {
		t.Fatal(err)
	}
	if _, err := chSvc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	sent, err := chSvc.Send(ctx, 1, ch.Id, 0, []channel.Outgoing{{RandomID: 3, Message: &mtproto.Message{Message: "post", Date: 5}}})
	if err != nil {
		t.Fatal(err)
	}
	postID := sent.Updates[0].GetMessage_MESSAGE().GetId()

	channelCore := &MessagesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 2},
	}
	got, err := channelCore.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer:      mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: ch.Id}).To_InputPeer(),
		Id:        []int32{postID},
		Increment: mtproto.BoolTrue,
	})
	if err != nil || len(got.GetViews()) != 1 || got.GetViews()[0].GetViews().GetValue() != 2 {
		t.Fatalf("channel views %+v %v", got, err)
	}

	fwdBox := func(id int32) *mtproto.MessageBox {
		return &mtproto.MessageBox{
			MessageId: id,
			Message: mtproto.MakeTLMessage(&mtproto.Message{
				Id: id,
				FwdFrom: mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
					FromId:      mtproto.MakePeerChannel(ch.Id),
					ChannelPost: wrapperspb.Int32(postID),
					Date:        5,
				}).To_MessageFwdHeader(),
			}).To_Message(),
		}
	}
	userCore := &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient:    viewUsers{},
			MessageClient: &viewMessages{boxes: []*mtproto.MessageBox{fwdBox(7), fwdBox(8)}},
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 10},
	}
	// Two local ids share one forwarded post, so the counter moves once.
	fwd, err := userCore.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer:      mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 20}).To_InputPeer(),
		Id:        []int32{7, 8},
		Increment: mtproto.BoolTrue,
	})
	if err != nil || len(fwd.GetViews()) != 2 {
		t.Fatalf("forward %+v %v", fwd, err)
	}
	if fwd.GetViews()[0].GetViews().GetValue() != 3 || fwd.GetViews()[1].GetViews().GetValue() != 3 {
		t.Fatalf("forward counts %+v", fwd.GetViews())
	}
}

type viewUsers struct {
	user_client.UserClient
}

func (viewUsers) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		{User: &mtproto.UserData{Id: 10}},
		{User: &mtproto.UserData{Id: 20}},
	}}, nil
}

type viewMessages struct {
	message_client.MessageClient
	boxes []*mtproto.MessageBox
}

func (m *viewMessages) MessageGetUserMessageList(context.Context, *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	return &messagepb.Vector_MessageBox{Datas: m.boxes}, nil
}
