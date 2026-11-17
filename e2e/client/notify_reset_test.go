package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestResetNotifySettingsDropsCachedMute is account.resetNotifySettings.
// The database soft-delete used to leave user_notify_settings_v2_* in
// Redis, so getNotifySettings kept the old mute while getPeerDialogs,
// which reads the table, already showed the default. Peer, global and
// story fields are the same cached object.
func TestResetNotifySettingsDropsCachedMute(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileAndroid, "notify_reset", "Reset")
	_, _ = loginNamed(t, cfg, ProfileAndroid, "notify_reset_peer", "Peer")
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()

	global := mtproto.MakeTLInputNotifyUsers(&mtproto.InputNotifyPeer{}).To_InputNotifyPeer()
	peer := mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{
		Peer: userPeer(t, cfg, c, "notify_reset_peer", "Peer"),
	}).To_InputNotifyPeer()
	muted := mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
		MuteUntil:    wrapperspb.Int32(2147483647),
		StoriesMuted: mtproto.BoolTrue,
	}).To_InputPeerNotifySettings()
	for _, target := range []*mtproto.InputNotifyPeer{global, peer} {
		invokeOK(t, c, &mtproto.TLAccountUpdateNotifySettings{
			Constructor: mtproto.CRC32_account_updateNotifySettings,
			Peer:        target,
			Settings:    muted,
		})
	}
	if notifySettings(t, ctx, c, global).GetMuteUntil().GetValue() != 2147483647 ||
		!mtproto.FromBool(notifySettings(t, ctx, c, global).GetStoriesMuted()) {
		t.Fatal("global mute was not stored")
	}
	if notifySettings(t, ctx, c, peer).GetMuteUntil().GetValue() != 2147483647 {
		t.Fatal("peer mute was not stored")
	}

	reset := invokeOK(t, c, &mtproto.TLAccountResetNotifySettings{
		Constructor: mtproto.CRC32_account_resetNotifySettings,
	})
	if _, ok := reset.(*mtproto.TLBoolTrue); !ok {
		t.Fatalf("reset %T", reset)
	}

	gotGlobal := notifySettings(t, ctx, c, global)
	if gotGlobal.GetMuteUntil().GetValue() == 2147483647 || mtproto.FromBool(gotGlobal.GetStoriesMuted()) {
		t.Fatalf("global notify settings stayed muted: %+v", gotGlobal)
	}
	gotPeer := notifySettings(t, ctx, c, peer)
	if gotPeer.GetMuteUntil().GetValue() == 2147483647 || mtproto.FromBool(gotPeer.GetStoriesMuted()) {
		t.Fatalf("peer notify settings stayed muted: %+v", gotPeer)
	}
}
