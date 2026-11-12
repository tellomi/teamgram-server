package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

func TestPeerNotifySettingsFromDOKeepsLegacySoundAndNewColumns(t *testing.T) {
	legacy := PeerNotifySettingsFromDO(&dataobject.UserNotifySettingsDO{
		ShowPreviews: -1,
		Silent:       -1,
		MuteUntil:    2147483647,
		Sound:        "default",
		StoriesMuted: -1,
	})
	if legacy.GetMuteUntil().GetValue() != 2147483647 {
		t.Fatalf("mute %d", legacy.GetMuteUntil().GetValue())
	}
	if legacy.GetShowPreviews() != nil || legacy.GetSilent() != nil {
		t.Fatal("unset flags were returned")
	}
	if legacy.GetAndroidSound().GetPredicateName() != mtproto.Predicate_notificationSoundDefault ||
		legacy.GetIosSound().GetPredicateName() != mtproto.Predicate_notificationSoundDefault {
		t.Fatal("a legacy sound must still be visible to current clients")
	}

	stored := PeerNotifySettingsFromDO(&dataobject.UserNotifySettingsDO{
		ShowPreviews:        1,
		Silent:              0,
		MuteUntil:           0,
		Sound:               "42",
		StoriesMuted:        1,
		StoriesHideSender:   0,
		AndroidSound:        "ringtone:42",
		StoriesAndroidSound: "none",
		IosSound:            "",
	})
	if mtproto.FromBool(stored.GetSilent()) || !mtproto.FromBool(stored.GetStoriesMuted()) || mtproto.FromBool(stored.GetStoriesHideSender()) {
		t.Fatalf("flags silent=%v stories=%v hide=%v", stored.GetSilent(), stored.GetStoriesMuted(), stored.GetStoriesHideSender())
	}
	if stored.GetAndroidSound().GetId() != 42 {
		t.Fatalf("android sound %+v", stored.GetAndroidSound())
	}
	if stored.GetIosSound() != nil {
		t.Fatal("an empty ios_sound must stay unset once a platform column is set")
	}
	if stored.GetStoriesAndroidSound().GetPredicateName() != mtproto.Predicate_notificationSoundNone {
		t.Fatalf("stories sound %+v", stored.GetStoriesAndroidSound())
	}
}
