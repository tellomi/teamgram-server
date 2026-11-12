package mtproto

import (
	"testing"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDefaultPeerNotifySettingsAreNotMuted(t *testing.T) {
	got := MakeDefaultPeerNotifySettings()
	if !FromBool(got.GetShowPreviews()) {
		t.Fatal("show_previews default")
	}
	if FromBool(got.GetSilent()) {
		t.Fatal("silent default is inverted")
	}
	if got.GetMuteUntil() == nil || got.GetMuteUntil().GetValue() != 0 {
		t.Fatalf("mute_until %#v", got.GetMuteUntil())
	}
	if got.GetIosSound().GetPredicateName() != Predicate_notificationSoundDefault ||
		got.GetAndroidSound().GetPredicateName() != Predicate_notificationSoundDefault ||
		got.GetOtherSound().GetPredicateName() != Predicate_notificationSoundDefault {
		t.Fatal("default sound was not notificationSoundDefault on every platform")
	}
}

func TestMakePeerNotifySettingsKeepsSoundAndStories(t *testing.T) {
	local := MakeTLNotificationSoundLocal(&NotificationSound{Title: "Glass", Data: "glass.caf"}).To_NotificationSound()
	stories := MakeTLNotificationSoundRingtone(&NotificationSound{Id: 42}).To_NotificationSound()
	got, err := MakePeerNotifySettings(MakeTLInputPeerNotifySettings(&InputPeerNotifySettings{
		ShowPreviews:                BoolFalse,
		Silent:                      BoolTrue,
		MuteUntil:                   &wrapperspb.Int32Value{Value: 2147483647},
		Sound_FLAGNOTIFICATIONSOUND: local,
		StoriesMuted:                BoolTrue,
		StoriesHideSender:           BoolFalse,
		StoriesSound:                stories,
	}).To_InputPeerNotifySettings())
	if err != nil {
		t.Fatal(err)
	}
	if FromBool(got.GetShowPreviews()) || !FromBool(got.GetSilent()) || got.GetMuteUntil().GetValue() != 2147483647 {
		t.Fatalf("flags %#v", got)
	}
	for _, sound := range []*NotificationSound{got.GetIosSound(), got.GetAndroidSound(), got.GetOtherSound()} {
		if sound.GetPredicateName() != Predicate_notificationSoundLocal || sound.GetTitle() != "Glass" || sound.GetData() != "glass.caf" {
			t.Fatalf("platform sound %#v", sound)
		}
	}
	if got.GetStoriesIosSound().GetPredicateName() != Predicate_notificationSoundRingtone || got.GetStoriesIosSound().GetId() != 42 {
		t.Fatalf("stories sound %#v", got.GetStoriesIosSound())
	}
	if !FromBool(got.GetStoriesMuted()) || FromBool(got.GetStoriesHideSender()) {
		t.Fatal("stories flags")
	}
	if LegacySound(got.GetIosSound()) != "local:Glass" {
		t.Fatalf("legacy %s", LegacySound(got.GetIosSound()))
	}
	back := SoundFromLegacy("local:Glass")
	if back.GetPredicateName() != Predicate_notificationSoundLocal || back.GetTitle() != "Glass" {
		t.Fatalf("legacy parse %#v", back)
	}
	if SoundFromLegacy("42").GetId() != 42 || SoundFromLegacy("none").GetPredicateName() != Predicate_notificationSoundNone {
		t.Fatal("legacy ringtone or none")
	}
}
