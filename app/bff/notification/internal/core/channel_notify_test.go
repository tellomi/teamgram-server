package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestChannelForNotifyWithoutPlugin(t *testing.T) {
	prev := channel.Shared()
	chSvc := channel.New(channel.NewMemoryStore())
	channel.Use(chSvc)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	up, err := chSvc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	c := &NotificationCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
	chat, err := c.channelForNotify(up.Chats[0].GetId())
	if err != nil || chat.GetId() != up.Chats[0].GetId() || !chat.GetBroadcast() {
		t.Fatalf("%v %#v", err, chat)
	}
	if _, err := c.channelForNotify(999); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("missing channel: %v", err)
	}
}
