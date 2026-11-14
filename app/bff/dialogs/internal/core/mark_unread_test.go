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
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestMarkDialogUnreadPushesOtherSessions(t *testing.T) {
	ctx := context.Background()
	syncer := &unreadSync{}
	c := &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient: unreadDialogs{},
			SyncClient:   syncer,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 7, PermAuthKeyId: 11},
	}
	ok, err := c.MessagesMarkDialogUnread(&mtproto.TLMessagesMarkDialogUnread{
		Unread: true,
		Peer: mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
			Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer(),
		}).To_InputDialogPeer(),
	})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("mark %v %v", ok, err)
	}
	if syncer.notMe == nil || syncer.notMe.GetUserId() != 7 || syncer.notMe.GetPermAuthKeyId() != 11 {
		t.Fatalf("push %+v", syncer.notMe)
	}
	ups := syncer.notMe.GetUpdates().GetUpdates()
	if len(ups) != 1 || ups[0].GetPredicateName() != mtproto.Predicate_updateDialogUnreadMark || !ups[0].GetUnread() {
		t.Fatalf("update %+v", ups)
	}
	if ups[0].GetPeer_DIALOGPEER().GetPeer().GetUserId() != 8 {
		t.Fatalf("peer %+v", ups[0].GetPeer_DIALOGPEER())
	}
}

type unreadDialogs struct{ dialogclient.DialogClient }

func (unreadDialogs) DialogMarkDialogUnread(context.Context, *dialog.TLDialogMarkDialogUnread) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

type unreadSync struct {
	syncclient.SyncClient
	notMe *sync.TLSyncUpdatesNotMe
}

func (s *unreadSync) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.notMe = in
	return mtproto.MakeTLVoid(nil).To_Void(), nil
}
