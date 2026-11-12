package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestUpdateNotifySettingsPushesTheStoredRow(t *testing.T) {
	local := mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{Id: 7}).To_NotificationSound()
	stored, err := mtproto.MakePeerNotifySettings(mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
		ShowPreviews:                mtproto.BoolFalse,
		Silent:                      mtproto.BoolFalse,
		MuteUntil:                   &wrapperspb.Int32Value{Value: 50},
		Sound_FLAGNOTIFICATIONSOUND: local,
		StoriesMuted:                mtproto.BoolTrue,
	}).To_InputPeerNotifySettings())
	if err != nil {
		t.Fatal(err)
	}
	users := &notifyUsers{readback: stored}
	syncer := &notifySync{}
	c := &NotificationCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: users,
			SyncClient: syncer,
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1, PermAuthKeyId: 3},
	}
	ok, err := c.AccountUpdateNotifySettings(&mtproto.TLAccountUpdateNotifySettings{
		Peer: mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{
			Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer(),
		}).To_InputNotifyPeer(),
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			MuteUntil: &wrapperspb.Int32Value{Value: 50},
		}).To_InputPeerNotifySettings(),
	})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("%v %v", ok, err)
	}
	if len(syncer.updates) != 1 {
		t.Fatalf("pushes %d", len(syncer.updates))
	}
	var found *mtproto.PeerNotifySettings
	for _, upd := range syncer.updates[0].GetUpdates() {
		if upd.GetPredicateName() == mtproto.Predicate_updateNotifySettings {
			found = upd.GetNotifySettings()
		}
	}
	if found == nil {
		t.Fatal("updateNotifySettings was not sent")
	}
	if found.GetMuteUntil().GetValue() != 50 || mtproto.FromBool(found.GetShowPreviews()) || mtproto.FromBool(found.GetSilent()) {
		t.Fatalf("pushed flags %#v", found)
	}
	if found.GetIosSound().GetPredicateName() != mtproto.Predicate_notificationSoundRingtone || found.GetIosSound().GetId() != 7 {
		t.Fatalf("pushed sound %#v", found.GetIosSound())
	}
	if !mtproto.FromBool(found.GetStoriesMuted()) {
		t.Fatal("stories_muted was not in the update")
	}
	if syncer.perm != 3 {
		t.Fatalf("perm auth key %d", syncer.perm)
	}
}

func TestUpdateNotifySettingsDoesNotPretendSyncSucceeded(t *testing.T) {
	users := &notifyUsers{readback: mtproto.MakeDefaultPeerNotifySettings()}
	syncer := &notifySync{err: errors.New("sync down")}
	c := &NotificationCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: users,
			SyncClient: syncer,
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1, PermAuthKeyId: 3},
	}
	ok, err := c.AccountUpdateNotifySettings(&mtproto.TLAccountUpdateNotifySettings{
		Peer: mtproto.MakeTLInputNotifyUsers(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer(),
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			MuteUntil: &wrapperspb.Int32Value{Value: 1},
		}).To_InputPeerNotifySettings(),
	})
	if err == nil || ok != nil {
		t.Fatalf("sync failure looked like success: %v %v", ok, err)
	}
}

type notifyUsers struct {
	userclient.UserClient
	readback *mtproto.PeerNotifySettings
}

func (u *notifyUsers) UserGetMutableUsers(context.Context, *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error) {
	return &user.Vector_ImmutableUser{}, nil
}

func (u *notifyUsers) UserSetNotifySettings(context.Context, *user.TLUserSetNotifySettings) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func (u *notifyUsers) UserGetNotifySettings(context.Context, *user.TLUserGetNotifySettings) (*mtproto.PeerNotifySettings, error) {
	return u.readback, nil
}

type notifySync struct {
	syncclient.SyncClient
	updates []*mtproto.Updates
	perm    int64
	err     error
}

func (s *notifySync) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.perm = in.PermAuthKeyId
	if in.Updates != nil {
		s.updates = append(s.updates, in.Updates)
	}
	if s.err != nil {
		return nil, s.err
	}
	return mtproto.EmptyVoid, nil
}
