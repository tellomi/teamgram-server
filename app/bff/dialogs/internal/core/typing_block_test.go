package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestSetTypingSkippedWhenBlocked(t *testing.T) {
	users := &typingUser{}
	pushes := &typingSync{}
	ctx := context.Background()
	c := &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: users,
			SyncClient: pushes,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
	in := &mtproto.TLMessagesSetTyping{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2, AccessHash: 1}).To_InputPeer(),
		Action: mtproto.MakeTLSendMessageTypingAction(&mtproto.SendMessageAction{}).To_SendMessageAction(),
	}

	users.blocked = true
	if _, err := c.MessagesSetTyping(in); err != nil {
		t.Fatal(err)
	}
	if pushes.n != 0 {
		t.Fatalf("blocked typing was pushed %d", pushes.n)
	}

	users.blocked = false
	if _, err := c.MessagesSetTyping(in); err != nil {
		t.Fatal(err)
	}
	if pushes.n != 1 || pushes.user != 2 {
		t.Fatalf("pushes %d user %d", pushes.n, pushes.user)
	}
}

func TestChannelTypingReachesOtherMembers(t *testing.T) {
	prev := channel.Shared()
	chsvc := channel.New(channel.NewMemoryStore())
	channel.Use(chsvc)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	created, err := chsvc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "type room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := &mtproto.InputChannel{
		PredicateName: mtproto.Predicate_inputChannel,
		ChannelId:     ch.Id,
		AccessHash:    ch.GetAccessHash_FLAGINT64().GetValue(),
	}
	if _, err := chsvc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	pushes := &typingSync{}
	c := &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			SyncClient: pushes,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
	if _, err := c.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
		Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
			ChannelId:  ch.Id,
			AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
		}).To_InputPeer(),
		TopMsgId: &wrapperspb.Int32Value{Value: 7},
		Action:   mtproto.MakeTLSendMessageTypingAction(&mtproto.SendMessageAction{}).To_SendMessageAction(),
	}); err != nil {
		t.Fatal(err)
	}
	if pushes.n != 2 {
		t.Fatalf("member pushes %d", pushes.n)
	}
	if pushes.update.GetChannelId() != ch.Id || pushes.update.GetTopMsgId_FLAGINT32().GetValue() != 7 {
		t.Fatalf("update %#v", pushes.update)
	}
	if pushes.update.GetPredicateName() != mtproto.Predicate_updateChannelUserTyping {
		t.Fatalf("predicate %s", pushes.update.GetPredicateName())
	}
}

type typingUser struct {
	userclient.UserClient
	blocked bool
}

func (u *typingUser) UserBlockedEitherWay(context.Context, *user.TLUserCheckBlockUserList) (*user.Vector_Long, error) {
	if u.blocked {
		return &user.Vector_Long{Datas: []int64{2}}, nil
	}
	return &user.Vector_Long{}, nil
}

type typingSync struct {
	syncclient.SyncClient
	n      int
	user   int64
	update *mtproto.Update
}

func (s *typingSync) SyncPushUpdates(_ context.Context, in *sync.TLSyncPushUpdates) (*mtproto.Void, error) {
	s.n++
	s.user = in.GetUserId()
	if in.GetUpdates() != nil {
		s.update = in.GetUpdates().GetUpdate()
	}
	return mtproto.EmptyVoid, nil
}
