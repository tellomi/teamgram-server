package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestNotifySettingsStickAndReachTheOtherSession stores the current
// inputPeerNotifySettings fields and checks that another session of the same
// user receives updateNotifySettings. The global default is not silent.
func TestNotifySettingsStickAndReachTheOtherSession(t *testing.T) {
	cfg := requireE2E(t)
	android, _ := loginNamed(t, cfg, ProfileAndroid, "notify_a", "NotifyA")
	ios, _ := loginNamed(t, cfg, ProfileIOS, "notify_a", "NotifyA")
	// The contact has to exist before importContacts will return them.
	_, _ = loginNamed(t, cfg, ProfileAndroid, "notify_b", "NotifyB")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	global := notifySettings(t, ctx, android, mtproto.MakeTLInputNotifyUsers(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer())
	if mtproto.FromBool(global.GetSilent()) {
		t.Fatal("default silent is true")
	}

	peer := userPeer(t, cfg, android, "notify_b", "NotifyB")
	target := mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{Peer: peer}).To_InputNotifyPeer()
	invokeOK(t, android, &mtproto.TLAccountUpdateNotifySettings{
		Constructor: mtproto.CRC32_account_updateNotifySettings,
		Peer:        target,
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			ShowPreviews:                mtproto.BoolFalse,
			Silent:                      mtproto.BoolFalse,
			MuteUntil:                   wrapperspb.Int32(2147483647),
			StoriesMuted:                mtproto.BoolTrue,
			StoriesHideSender:           mtproto.BoolTrue,
			Sound_FLAGNOTIFICATIONSOUND: mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{Id: 42}).To_NotificationSound(),
			StoriesSound:                mtproto.MakeTLNotificationSoundNone(nil).To_NotificationSound(),
		}).To_InputPeerNotifySettings(),
	})

	got := notifySettings(t, ctx, android, target)
	if got.GetMuteUntil().GetValue() != 2147483647 || mtproto.FromBool(got.GetShowPreviews()) {
		t.Fatalf("stored mute/preview %+v", got)
	}
	if !mtproto.FromBool(got.GetStoriesMuted()) || !mtproto.FromBool(got.GetStoriesHideSender()) {
		t.Fatalf("stories flags %+v %+v", got.GetStoriesMuted(), got.GetStoriesHideSender())
	}
	if got.GetAndroidSound().GetPredicateName() != mtproto.Predicate_notificationSoundRingtone || got.GetAndroidSound().GetId() != 42 {
		t.Fatalf("android sound %+v", got.GetAndroidSound())
	}
	if got.GetStoriesAndroidSound().GetPredicateName() != mtproto.Predicate_notificationSoundNone {
		t.Fatalf("stories sound %+v", got.GetStoriesAndroidSound())
	}

	if err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		for _, obj := range ios.Pushed() {
			for _, upd := range updatesOf(obj) {
				if upd.GetPredicateName() != mtproto.Predicate_updateNotifySettings {
					continue
				}
				s := upd.GetNotifySettings()
				if s.GetMuteUntil().GetValue() == 2147483647 && s.GetAndroidSound().GetId() == 42 {
					return true, nil
				}
			}
		}
		return false, nil
	}); err != nil {
		t.Fatalf("other session did not receive updateNotifySettings: %v", err)
	}

	again := notifySettings(t, ctx, ios, target)
	if again.GetMuteUntil().GetValue() != 2147483647 || again.GetAndroidSound().GetId() != 42 || !mtproto.FromBool(again.GetStoriesMuted()) {
		t.Fatalf("other session getNotifySettings %+v", again)
	}
}

func notifySettings(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputNotifyPeer) *mtproto.PeerNotifySettings {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLAccountGetNotifySettings{
		Constructor: mtproto.CRC32_account_getNotifySettings,
		Peer:        peer,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.PeerNotifySettings:
		return v
	case *mtproto.TLPeerNotifySettings:
		return v.To_PeerNotifySettings()
	default:
		t.Fatalf("getNotifySettings %T", obj)
		return nil
	}
}
