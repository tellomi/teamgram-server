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
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
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
	n    int
	user int64
}

func (s *typingSync) SyncPushUpdates(_ context.Context, in *sync.TLSyncPushUpdates) (*mtproto.Void, error) {
	s.n++
	s.user = in.GetUserId()
	return mtproto.EmptyVoid, nil
}
