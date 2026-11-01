package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestGetOnlinesCountsOpenPresenceWindow(t *testing.T) {
	now := time.Now().Unix()
	users := &onlineUsers{rows: []*userpb.LastSeenData{
		{UserId: 10, LastSeenAt: now, Expires: 300},
		{UserId: 20, LastSeenAt: now, Expires: 0},
		{UserId: 30, LastSeenAt: now - 1000, Expires: 300},
	}}
	c := newOnlineCore(&onlineChat{ids: []int64{10, 20, 30}}, users)
	got, err := c.MessagesGetOnlines(&mtproto.TLMessagesGetOnlines{
		Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 5}).To_InputPeer(),
	})
	if err != nil || got.GetOnlines() != 1 {
		t.Fatalf("onlines %v %v", got, err)
	}
	if _, err := c.MessagesGetOnlines(&mtproto.TLMessagesGetOnlines{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 20}).To_InputPeer(),
	}); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("private chat: %v", err)
	}
}

func TestGetOnlinesChannelMembers(t *testing.T) {
	prev := channel.Shared()
	svc := channel.New(channel.NewMemoryStore())
	channel.Use(svc)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	up, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 10, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, 10, 0, channelIn(up.Chats[0]), []int64{20}, nil); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	users := &onlineUsers{rows: []*userpb.LastSeenData{
		{UserId: 10, LastSeenAt: now, Expires: 300},
		{UserId: 20, LastSeenAt: now - 10, Expires: 300},
	}}
	c := newOnlineCore(&onlineChat{}, users)
	got, err := c.MessagesGetOnlines(&mtproto.TLMessagesGetOnlines{
		Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: up.Chats[0].Id}).To_InputPeer(),
	})
	if err != nil || got.GetOnlines() != 2 {
		t.Fatalf("channel onlines %v %v ids %v", got, err, users.asked)
	}
}

func TestScreenshotNotificationServiceMessage(t *testing.T) {
	msg := &shotMsg{}
	c := &DialogsCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MsgClient: msg,
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 10, PermAuthKeyId: 99},
	}
	ups, err := c.MessagesSendScreenshotNotification(&mtproto.TLMessagesSendScreenshotNotification{
		Peer:         mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 20}).To_InputPeer(),
		ReplyToMsgId: 4,
		RandomId:     77,
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.last == nil || msg.last.UserId != 10 || msg.last.PeerId != 20 || msg.last.AuthKeyId != 99 {
		t.Fatalf("send %+v", msg.last)
	}
	sent := msg.last.GetMessage()[0].GetMessage()
	if sent.GetPredicateName() != mtproto.Predicate_messageService || sent.GetAction().GetPredicateName() != mtproto.Predicate_messageActionScreenshotTaken {
		t.Fatalf("action %#v", sent.GetAction())
	}
	if sent.GetReplyTo().GetReplyToMsgId() != 4 || msg.last.GetMessage()[0].GetRandomId() != 77 {
		t.Fatalf("reply %d random %d", sent.GetReplyTo().GetReplyToMsgId(), msg.last.GetMessage()[0].GetRandomId())
	}
	if ups == nil || len(ups.GetUpdates()) == 0 {
		t.Fatal("empty updates")
	}
	if _, err := c.MessagesSendScreenshotNotification(&mtproto.TLMessagesSendScreenshotNotification{
		Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 3}).To_InputPeer(),
	}); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("group: %v", err)
	}
}

func newOnlineCore(ch chat_client.ChatClient, users user_client.UserClient) *DialogsCore {
	ctx := context.Background()
	return &DialogsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{ChatClient: ch, UserClient: users}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 10},
	}
}

func channelIn(ch *mtproto.Chat) *mtproto.InputChannel {
	return mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  ch.Id,
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
}

type onlineChat struct {
	chat_client.ChatClient
	ids []int64
}

func (c *onlineChat) ChatGetChatParticipantIdList(_ context.Context, _ *chatpb.TLChatGetChatParticipantIdList) (*chatpb.Vector_Long, error) {
	return &chatpb.Vector_Long{Datas: c.ids}, nil
}

type onlineUsers struct {
	user_client.UserClient
	rows  []*userpb.LastSeenData
	asked []int64
}

func (u *onlineUsers) UserGetLastSeens(_ context.Context, in *userpb.TLUserGetLastSeens) (*userpb.Vector_LastSeenData, error) {
	u.asked = append([]int64(nil), in.GetId()...)
	return &userpb.Vector_LastSeenData{Datas: u.rows}, nil
}

type shotMsg struct {
	msg_client.MsgClient
	last *msgpb.TLMsgSendMessageV2
}

func (m *shotMsg) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	m.last = in
	box := in.GetMessage()[0]
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Pts_INT32:       3,
		PtsCount:        1,
		Message_MESSAGE: box.GetMessage(),
	}).To_Update()), nil
}
