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

func TestUpdateNotifySettingsPersistsSoundAndPushesOtherSessions(t *testing.T) {
	users := &notifyUserFake{stored: mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		MuteUntil:    wrapperspb.Int32(2147483647),
		AndroidSound: mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{Id: 42}).To_NotificationSound(),
		StoriesMuted: mtproto.BoolTrue,
	}).To_PeerNotifySettings()}
	pushes := &notifySyncFake{}
	c := notifyCore(users, pushes, "android")
	ok, err := c.AccountUpdateNotifySettings(&mtproto.TLAccountUpdateNotifySettings{
		Peer: mtproto.MakeTLInputNotifyUsers(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer(),
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			MuteUntil:                   wrapperspb.Int32(2147483647),
			StoriesMuted:                mtproto.BoolTrue,
			StoriesHideSender:           mtproto.BoolFalse,
			Sound_FLAGNOTIFICATIONSOUND: mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{Id: 42}).To_NotificationSound(),
			StoriesSound:                mtproto.MakeTLNotificationSoundNone(nil).To_NotificationSound(),
		}).To_InputPeerNotifySettings(),
	})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("update: %v %v", ok, err)
	}
	if users.saved == nil || users.saved.GetAndroidSound().GetId() != 42 {
		t.Fatalf("saved sound %+v", users.saved.GetAndroidSound())
	}
	if users.saved.GetIosSound() != nil {
		t.Fatal("android client wrote ios_sound")
	}
	if users.saved.GetStoriesAndroidSound().GetPredicateName() != mtproto.Predicate_notificationSoundNone {
		t.Fatalf("stories sound %+v", users.saved.GetStoriesAndroidSound())
	}
	if len(pushes.got) != 1 || pushes.got[0].GetPermAuthKeyId() != 11 {
		t.Fatalf("push %#v", pushes.got)
	}
	var saw bool
	for _, upd := range pushes.got[0].GetUpdates().GetUpdates() {
		if upd.GetPredicateName() != mtproto.Predicate_updateNotifySettings {
			continue
		}
		saw = true
		if upd.GetNotifySettings().GetMuteUntil().GetValue() != 2147483647 || upd.GetNotifySettings().GetAndroidSound().GetId() != 42 {
			t.Fatalf("pushed settings %+v", upd.GetNotifySettings())
		}
	}
	if !saw {
		t.Fatal("other sessions were not sent updateNotifySettings")
	}
}

func TestUpdateNotifySettingsDoesNotClaimSuccessWhenPushFails(t *testing.T) {
	users := &notifyUserFake{stored: mtproto.MakeDefaultPeerNotifySettings()}
	pushes := &notifySyncFake{err: errors.New("sync down")}
	c := notifyCore(users, pushes, "ios")
	_, err := c.AccountUpdateNotifySettings(&mtproto.TLAccountUpdateNotifySettings{
		Peer: mtproto.MakeTLInputNotifyUsers(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer(),
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			Silent: mtproto.BoolTrue,
		}).To_InputPeerNotifySettings(),
	})
	if err == nil {
		t.Fatal("sync failure was returned as success")
	}
}

func notifyCore(users userclient.UserClient, pushes syncclient.SyncClient, langpack string) *NotificationCore {
	ctx := context.Background()
	return &NotificationCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: users, SyncClient: pushes}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 7, PermAuthKeyId: 11, Langpack: langpack},
	}
}

type notifyUserFake struct {
	userclient.UserClient
	saved  *mtproto.PeerNotifySettings
	stored *mtproto.PeerNotifySettings
	setReq *user.TLUserSetNotifySettings
	getReq *user.TLUserGetNotifySettings
	all    *user.Vector_PeerPeerNotifySettings
}

func (f *notifyUserFake) UserSetNotifySettings(_ context.Context, in *user.TLUserSetNotifySettings) (*mtproto.Bool, error) {
	f.setReq = in
	f.saved = in.GetSettings()
	return mtproto.BoolTrue, nil
}

func (f *notifyUserFake) UserGetNotifySettings(_ context.Context, in *user.TLUserGetNotifySettings) (*mtproto.PeerNotifySettings, error) {
	f.getReq = in
	return f.stored, nil
}

func (f *notifyUserFake) UserGetAllNotifySettings(context.Context, *user.TLUserGetAllNotifySettings) (*user.Vector_PeerPeerNotifySettings, error) {
	if f.all == nil {
		return &user.Vector_PeerPeerNotifySettings{}, nil
	}
	return f.all, nil
}

type notifySyncFake struct {
	syncclient.SyncClient
	got []*sync.TLSyncUpdatesNotMe
	err error
}

func (f *notifySyncFake) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	f.got = append(f.got, in)
	if f.err != nil {
		return nil, f.err
	}
	return mtproto.EmptyVoid, nil
}
