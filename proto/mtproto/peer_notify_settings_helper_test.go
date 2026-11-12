package mtproto

import (
	"testing"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDefaultNotifySilentIsFalse(t *testing.T) {
	s := MakeDefaultPeerNotifySettings()
	if FromBool(s.GetSilent()) {
		t.Fatal("default silent is true; notifications would be silent until a client opts in")
	}
	if !FromBool(s.GetShowPreviews()) || s.GetMuteUntil().GetValue() != 0 {
		t.Fatalf("preview/mute default: %+v", s)
	}
	if s.GetAndroidSound().GetPredicateName() != Predicate_notificationSoundDefault {
		t.Fatalf("default android sound %s", s.GetAndroidSound().GetPredicateName())
	}
	if FromBool(s.GetStoriesMuted()) || FromBool(s.GetStoriesHideSender()) {
		t.Fatal("stories are muted or hidden by default")
	}
}

func TestApplyNotifySettingsKeepsUnsetFieldsAndStoresThePlatformSound(t *testing.T) {
	dst := MakeTLPeerNotifySettings(&PeerNotifySettings{
		ShowPreviews: ToBool(true),
		MuteUntil:    wrapperspb.Int32(50),
	}).To_PeerNotifySettings()
	in := MakeTLInputPeerNotifySettings(&InputPeerNotifySettings{
		MuteUntil:                   wrapperspb.Int32(2147483647),
		StoriesMuted:                BoolTrue,
		StoriesSound:                MakeTLNotificationSoundNone(nil).To_NotificationSound(),
		Sound_FLAGNOTIFICATIONSOUND: MakeTLNotificationSoundRingtone(&NotificationSound{Id: 42}).To_NotificationSound(),
	}).To_InputPeerNotifySettings()
	ApplyInputPeerNotifySettings(dst, in, NotifySoundSlot("android", ""))
	if dst.GetMuteUntil().GetValue() != 2147483647 {
		t.Fatalf("mute %d", dst.GetMuteUntil().GetValue())
	}
	if !FromBool(dst.GetShowPreviews()) {
		t.Fatal("show_previews was cleared even though the input did not set it")
	}
	if dst.GetAndroidSound().GetPredicateName() != Predicate_notificationSoundRingtone || dst.GetAndroidSound().GetId() != 42 {
		t.Fatalf("android sound %+v", dst.GetAndroidSound())
	}
	if dst.GetIosSound() != nil {
		t.Fatal("an android update wrote ios_sound")
	}
	if dst.GetStoriesAndroidSound().GetPredicateName() != Predicate_notificationSoundNone {
		t.Fatalf("stories sound %+v", dst.GetStoriesAndroidSound())
	}
	cols := NotifyColumnUpdates(dst)
	if _, ok := cols["show_previews"]; !ok {
		t.Fatal("show_previews should be stored")
	}
	if cols["mute_until"] != int32(2147483647) {
		t.Fatalf("mute column %v", cols["mute_until"])
	}
	if cols["android_sound"] != "ringtone:42" || cols["stories_android_sound"] != "none" {
		t.Fatalf("columns %v", cols)
	}
	if _, ok := cols["ios_sound"]; ok {
		t.Fatal("unset ios_sound must not be written")
	}
	sparse := NotifyColumnUpdates(MakeTLPeerNotifySettings(&PeerNotifySettings{
		MuteUntil: wrapperspb.Int32(5),
	}).To_PeerNotifySettings())
	if len(sparse) != 1 || sparse["mute_until"] != int32(5) {
		t.Fatalf("partial map %v", sparse)
	}
}

func TestNotificationSoundRoundTrip(t *testing.T) {
	local := MakeTLNotificationSoundLocal(&NotificationSound{Title: "Glass", Data: "glass.caf"}).To_NotificationSound()
	again := DecodeNotificationSound(EncodeNotificationSound(local))
	if again.GetTitle() != "Glass" || again.GetData() != "glass.caf" {
		t.Fatalf("%+v", again)
	}
	if DecodeNotificationSound("default").GetPredicateName() != Predicate_notificationSoundDefault {
		t.Fatal("default")
	}
	if DecodeNotificationSound("none").GetPredicateName() != Predicate_notificationSoundNone {
		t.Fatal("none")
	}
	if DecodeNotificationSound("99").GetId() != 99 {
		t.Fatal("legacy ringtone id")
	}
	if DecodeNotificationSound("") != nil || DecodeNotificationSound("-1") != nil {
		t.Fatal("unset sound decoded to a value")
	}
	if NotifySoundSlot("ios", "iPhone") != "ios" || NotifySoundSlot("tdesktop", "") != "other" {
		t.Fatal("slot")
	}
}
