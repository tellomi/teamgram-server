package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestNotifySettingsPersistAndSync checks the documented defaults, then a
// mute plus a ringtone and stories fields. The same account on another
// device receives updateNotifySettings and getNotifySettings reads the same
// mute_until, show_previews and silent.
func TestNotifySettingsPersistAndSync(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "notify_a", "NotifyA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "notify_b", "NotifyB")
	a2, _ := loginNamed(t, cfg, ProfileAndroid, "notify_a", "NotifyA")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// A session only receives live updates after updates.getState,
	// getDifference, or account.updateStatus. Until then it is not generic,
	// it is not in the online-session list, and sync.updatesNotMe has
	// nowhere to write updateNotifySettings. Official clients call getState
	// after login; the other push cases in this package do the same.
	if _, _, _, err := a2.State(ctx); err != nil {
		t.Fatal(err)
	}

	def := getNotify(t, ctx, a, mtproto.MakeTLInputNotifyUsers(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer())
	if !mtproto.FromBool(def.GetShowPreviews()) || mtproto.FromBool(def.GetSilent()) || def.GetMuteUntil().GetValue() != 0 {
		t.Fatalf("default show_previews/silent/mute_until = %v %v %v", mtproto.FromBool(def.GetShowPreviews()), mtproto.FromBool(def.GetSilent()), def.GetMuteUntil())
	}

	peer := mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{
		Peer: inputPeerUser(bSelf.ID, bSelf.AccessHash),
	}).To_InputNotifyPeer()
	before := len(a2.Pushed())
	ringtone := mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{Id: 15}).To_NotificationSound()
	stories := mtproto.MakeTLNotificationSoundNone(&mtproto.NotificationSound{}).To_NotificationSound()
	setNotify(t, ctx, a, peer, mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
		ShowPreviews:                mtproto.BoolFalse,
		Silent:                      mtproto.BoolTrue,
		MuteUntil:                   &wrapperspb.Int32Value{Value: 2147483647},
		Sound_FLAGNOTIFICATIONSOUND: ringtone,
		StoriesMuted:                mtproto.BoolTrue,
		StoriesHideSender:           mtproto.BoolFalse,
		StoriesSound:                stories,
	}).To_InputPeerNotifySettings())

	want := func(got *mtproto.PeerNotifySettings) {
		t.Helper()
		if mtproto.FromBool(got.GetShowPreviews()) || !mtproto.FromBool(got.GetSilent()) || got.GetMuteUntil().GetValue() != 2147483647 {
			t.Fatalf("readback show_previews=%v silent=%v mute=%v", mtproto.FromBool(got.GetShowPreviews()), mtproto.FromBool(got.GetSilent()), got.GetMuteUntil())
		}
		if got.GetIosSound().GetPredicateName() != mtproto.Predicate_notificationSoundRingtone || got.GetIosSound().GetId() != 15 {
			t.Fatalf("sound %#v", got.GetIosSound())
		}
		if !mtproto.FromBool(got.GetStoriesMuted()) || mtproto.FromBool(got.GetStoriesHideSender()) {
			t.Fatal("stories flags")
		}
		if got.GetStoriesIosSound().GetPredicateName() != mtproto.Predicate_notificationSoundNone {
			t.Fatalf("stories sound %#v", got.GetStoriesIosSound())
		}
	}
	want(getNotify(t, ctx, a, peer))
	want(getNotify(t, ctx, a2, peer))

	deadline := time.Now().Add(15 * time.Second)
	for {
		if settings := pushedNotify(a2, before); settings != nil && settings.GetMuteUntil().GetValue() == 2147483647 && !mtproto.FromBool(settings.GetShowPreviews()) && mtproto.FromBool(settings.GetSilent()) {
			return
		}
		if time.Now().After(deadline) {
			if settings := pushedNotify(a2, before); settings != nil {
				t.Fatalf("updateNotifySettings arrived with show_previews=%v silent=%v mute=%v", mtproto.FromBool(settings.GetShowPreviews()), mtproto.FromBool(settings.GetSilent()), settings.GetMuteUntil())
			}
			t.Fatalf("the other device did not receive updateNotifySettings (%d new pushes)", len(a2.Pushed())-before)
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = b
}

func getNotify(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputNotifyPeer) *mtproto.PeerNotifySettings {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLAccountGetNotifySettings{
		Constructor: mtproto.CRC32_account_getNotifySettings,
		Peer:        peer,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLPeerNotifySettings:
		return v.To_PeerNotifySettings()
	case *mtproto.PeerNotifySettings:
		return v
	default:
		t.Fatalf("getNotifySettings %T", obj)
		return nil
	}
}

func setNotify(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputNotifyPeer, settings *mtproto.InputPeerNotifySettings) {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLAccountUpdateNotifySettings{
		Constructor: mtproto.CRC32_account_updateNotifySettings,
		Peer:        peer,
		Settings:    settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := obj.(*mtproto.TLBoolTrue); !ok {
		t.Fatalf("updateNotifySettings %T", obj)
	}
}

func pushedNotify(c *Client, skip int) *mtproto.PeerNotifySettings {
	pushed := c.Pushed()
	if skip > len(pushed) {
		skip = 0
	}
	for _, obj := range pushed[skip:] {
		var ups []*mtproto.Update
		switch v := obj.(type) {
		case *mtproto.TLUpdates:
			ups = v.GetUpdates()
		case *mtproto.TLUpdatesCombined:
			ups = v.GetUpdates()
		case *mtproto.TLUpdateShort:
			if v.GetUpdate() != nil {
				ups = []*mtproto.Update{v.GetUpdate()}
			}
		case *mtproto.Updates:
			ups = v.GetUpdates()
			if v.GetUpdate() != nil {
				ups = append(ups, v.GetUpdate())
			}
		}
		for _, upd := range ups {
			if upd.GetPredicateName() == mtproto.Predicate_updateNotifySettings && upd.GetNotifySettings() != nil {
				return upd.GetNotifySettings()
			}
		}
	}
	return nil
}
