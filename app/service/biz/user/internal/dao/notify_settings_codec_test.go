package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestSettingsFromRowDefaultsAreNotMuted(t *testing.T) {
	got := SettingsFromRow(mtproto.PEER_USERS, nil)
	if !mtproto.FromBool(got.GetShowPreviews()) || mtproto.FromBool(got.GetSilent()) || got.GetMuteUntil().GetValue() != 0 {
		t.Fatalf("global default %#v silent %v", got, mtproto.FromBool(got.GetSilent()))
	}
	if got.GetOtherSound().GetPredicateName() != mtproto.Predicate_notificationSoundDefault {
		t.Fatal("global default sound")
	}
	perChat := SettingsFromRow(mtproto.PEER_USER, nil)
	if perChat.GetShowPreviews() != nil || perChat.GetSilent() != nil || perChat.GetMuteUntil() != nil {
		t.Fatal("an unset chat must stay empty so the client inherits the global scope")
	}
}

func TestNotifySettingsRoundTripKeepsSoundWhenMuteChanges(t *testing.T) {
	local := mtproto.MakeTLNotificationSoundLocal(&mtproto.NotificationSound{Title: "Glass", Data: "glass.caf"}).To_NotificationSound()
	stories := mtproto.MakeTLNotificationSoundNone(&mtproto.NotificationSound{}).To_NotificationSound()
	first, err := mtproto.MakePeerNotifySettings(mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
		ShowPreviews:                mtproto.BoolTrue,
		Silent:                      mtproto.BoolFalse,
		MuteUntil:                   &wrapperspb.Int32Value{Value: 0},
		Sound_FLAGNOTIFICATIONSOUND: local,
		StoriesMuted:                mtproto.BoolTrue,
		StoriesHideSender:           mtproto.BoolFalse,
		StoriesSound:                stories,
	}).To_InputPeerNotifySettings())
	if err != nil {
		t.Fatal(err)
	}
	onlyMute, err := mtproto.MakePeerNotifySettings(mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
		MuteUntil: &wrapperspb.Int32Value{Value: 2147483647},
	}).To_InputPeerNotifySettings())
	if err != nil {
		t.Fatal(err)
	}
	merged := MergePeerNotify(first, onlyMute)
	do := doFromNotifyMap(makeDOByPeerNotifySettings(merged))
	back := PeerNotifyFromDO(do)
	if back.GetMuteUntil().GetValue() != 2147483647 || !mtproto.FromBool(back.GetShowPreviews()) || mtproto.FromBool(back.GetSilent()) {
		t.Fatalf("flags after mute %#v", back)
	}
	if back.GetIosSound().GetPredicateName() != mtproto.Predicate_notificationSoundLocal || back.GetIosSound().GetTitle() != "Glass" || back.GetIosSound().GetData() != "glass.caf" {
		t.Fatalf("sound was dropped %#v", back.GetIosSound())
	}
	if back.GetAndroidSound().GetPredicateName() != mtproto.Predicate_notificationSoundLocal || back.GetOtherSound().GetPredicateName() != mtproto.Predicate_notificationSoundLocal {
		t.Fatal("platform sounds diverged")
	}
	if !mtproto.FromBool(back.GetStoriesMuted()) || mtproto.FromBool(back.GetStoriesHideSender()) {
		t.Fatal("stories flags")
	}
	if back.GetStoriesIosSound().GetPredicateName() != mtproto.Predicate_notificationSoundNone {
		t.Fatalf("stories sound %#v", back.GetStoriesIosSound())
	}
	if do.Sound != "local:Glass" {
		t.Fatalf("legacy sound column %q", do.Sound)
	}
}

func doFromNotifyMap(m map[string]interface{}) *dataobject.UserNotifySettingsDO {
	asInt := func(key string) int32 {
		switch v := m[key].(type) {
		case int:
			return int32(v)
		case int32:
			return v
		default:
			return 0
		}
	}
	return &dataobject.UserNotifySettingsDO{
		ShowPreviews: asInt("show_previews"),
		Silent:       asInt("silent"),
		MuteUntil:    asInt("mute_until"),
		Sound:        m["sound"].(string),
		SettingsJSON: m["settings_json"].(string),
	}
}
