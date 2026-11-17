// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package mtproto

import (
	"encoding/json"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	DefaultPeerNotifySettings = MakeDefaultPeerNotifySettings()
)

// NotifySoundSlot is which peerNotifySettings sound an update writes.
// initConnection.lang_pack is "android" / "android_x", "ios", or something
// else (tdesktop, macos, web). Each client reads only its own slot.
func NotifySoundSlot(langpack, client string) string {
	s := strings.ToLower(langpack + " " + client)
	switch {
	case strings.Contains(s, "android"):
		return "android"
	case strings.Contains(s, "ios"):
		return "ios"
	default:
		return "other"
	}
}

// MakePeerNotifySettings copies the input fields the client actually set.
// slot is NotifySoundSlot. An unset flag is left nil so a later save does
// not wipe a value stored earlier.
func MakePeerNotifySettings(settings *InputPeerNotifySettings, slot string) (*PeerNotifySettings, error) {
	out := MakeTLPeerNotifySettings(&PeerNotifySettings{}).To_PeerNotifySettings()
	if settings == nil {
		return out, nil
	}
	ApplyInputPeerNotifySettings(out, settings, slot)
	return out, nil
}

// ApplyInputPeerNotifySettings merges one input onto dst.
// sound and stories_sound are stored on the platform slot. The legacy
// sound string is kept so older layers and push still see it.
func ApplyInputPeerNotifySettings(dst *PeerNotifySettings, in *InputPeerNotifySettings, slot string) {
	if dst == nil || in == nil {
		return
	}
	if in.ShowPreviews != nil {
		dst.ShowPreviews = in.ShowPreviews
	}
	if in.Silent != nil {
		dst.Silent = in.Silent
	}
	if in.MuteUntil != nil {
		dst.MuteUntil = in.MuteUntil
	}
	if in.StoriesMuted != nil {
		dst.StoriesMuted = in.StoriesMuted
	}
	if in.StoriesHideSender != nil {
		dst.StoriesHideSender = in.StoriesHideSender
	}
	sound := in.GetSound_FLAGNOTIFICATIONSOUND()
	if sound == nil && in.GetSound_FLAGSTRING() != nil {
		sound = DecodeNotificationSound(in.GetSound_FLAGSTRING().GetValue())
	}
	if sound != nil {
		setPlatformSound(dst, slot, sound, false)
		dst.Sound = &wrapperspb.StringValue{Value: LegacySoundString(sound)}
	}
	if in.GetStoriesSound() != nil {
		setPlatformSound(dst, slot, in.GetStoriesSound(), true)
	}
}

func setPlatformSound(dst *PeerNotifySettings, slot string, sound *NotificationSound, stories bool) {
	switch slot {
	case "android":
		if stories {
			dst.StoriesAndroidSound = sound
		} else {
			dst.AndroidSound = sound
		}
	case "ios":
		if stories {
			dst.StoriesIosSound = sound
		} else {
			dst.IosSound = sound
		}
	default:
		if stories {
			dst.StoriesOtherSound = sound
		} else {
			dst.OtherSound = sound
		}
	}
}

type localNotificationSound struct {
	Title string `json:"title"`
	Data  string `json:"data"`
}

// EncodeNotificationSound stores one of the four official sound constructors.
func EncodeNotificationSound(s *NotificationSound) string {
	if s == nil {
		return ""
	}
	switch s.GetPredicateName() {
	case Predicate_notificationSoundDefault:
		return "default"
	case Predicate_notificationSoundNone:
		return "none"
	case Predicate_notificationSoundRingtone:
		return "ringtone:" + strconv.FormatInt(s.GetId(), 10)
	case Predicate_notificationSoundLocal:
		b, err := json.Marshal(localNotificationSound{Title: s.GetTitle(), Data: s.GetData()})
		if err != nil {
			return ""
		}
		return "local:" + string(b)
	default:
		return ""
	}
}

// DecodeNotificationSound reads the stored form and the legacy sound string.
// "" and "-1" are unset. A bare integer is a ringtone id. Any other string
// is a local sound, which is what older clients stored.
func DecodeNotificationSound(raw string) *NotificationSound {
	switch raw {
	case "", "-1":
		return nil
	case "default":
		return MakeTLNotificationSoundDefault(&NotificationSound{}).To_NotificationSound()
	case "none":
		return MakeTLNotificationSoundNone(&NotificationSound{}).To_NotificationSound()
	}
	if strings.HasPrefix(raw, "ringtone:") {
		id, err := strconv.ParseInt(strings.TrimPrefix(raw, "ringtone:"), 10, 64)
		if err != nil {
			return nil
		}
		return MakeTLNotificationSoundRingtone(&NotificationSound{Id: id}).To_NotificationSound()
	}
	if strings.HasPrefix(raw, "local:") {
		var loc localNotificationSound
		if json.Unmarshal([]byte(strings.TrimPrefix(raw, "local:")), &loc) != nil {
			return nil
		}
		return MakeTLNotificationSoundLocal(&NotificationSound{Title: loc.Title, Data: loc.Data}).To_NotificationSound()
	}
	if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return MakeTLNotificationSoundRingtone(&NotificationSound{Id: id}).To_NotificationSound()
	}
	return MakeTLNotificationSoundLocal(&NotificationSound{Title: raw, Data: raw}).To_NotificationSound()
}

// LegacySoundString is the single sound column older clients and push read.
func LegacySoundString(s *NotificationSound) string {
	if s == nil {
		return ""
	}
	switch s.GetPredicateName() {
	case Predicate_notificationSoundDefault:
		return "default"
	case Predicate_notificationSoundNone:
		return "none"
	case Predicate_notificationSoundRingtone:
		return strconv.FormatInt(s.GetId(), 10)
	case Predicate_notificationSoundLocal:
		if s.GetData() != "" {
			return s.GetData()
		}
		return s.GetTitle()
	default:
		return ""
	}
}

// NotifyColumnUpdates is the partial UPDATE for one peerNotifySettings.
// Nil flags are omitted so they do not clear a previously stored value.
func NotifyColumnUpdates(s *PeerNotifySettings) map[string]interface{} {
	m := map[string]interface{}{}
	if s == nil {
		return m
	}
	if s.ShowPreviews != nil {
		m["show_previews"] = boolColumn(s.ShowPreviews)
	}
	if s.Silent != nil {
		m["silent"] = boolColumn(s.Silent)
	}
	if s.MuteUntil != nil {
		m["mute_until"] = s.MuteUntil.Value
	}
	if s.Sound != nil {
		m["sound"] = s.Sound.Value
	}
	if s.StoriesMuted != nil {
		m["stories_muted"] = boolColumn(s.StoriesMuted)
	}
	if s.StoriesHideSender != nil {
		m["stories_hide_sender"] = boolColumn(s.StoriesHideSender)
	}
	putSound := func(col string, sound *NotificationSound) {
		if sound != nil {
			m[col] = EncodeNotificationSound(sound)
		}
	}
	putSound("ios_sound", s.IosSound)
	putSound("android_sound", s.AndroidSound)
	putSound("other_sound", s.OtherSound)
	putSound("stories_ios_sound", s.StoriesIosSound)
	putSound("stories_android_sound", s.StoriesAndroidSound)
	putSound("stories_other_sound", s.StoriesOtherSound)
	return m
}

func boolColumn(v *Bool) int32 {
	if FromBool(v) {
		return 1
	}
	return 0
}

func notificationSoundDefault() *NotificationSound {
	return MakeTLNotificationSoundDefault(&NotificationSound{}).To_NotificationSound()
}

// MakeDefaultPeerNotifySettings
/*
   { updateNotifySettings
     peer: { notifyChats },
     notify_settings: { peerNotifySettings
       flags: 15 [INT],
       show_previews: { boolTrue },
       silent: { boolFalse },
       mute_until: 0 [INT],
       sound: "100" [STRING],
     },
   },
*/
func MakeDefaultPeerNotifySettings() *PeerNotifySettings {
	// silent is false. Notifications are not silent until a client sets the
	// flag. The previous value was the opposite of the constructor above.
	settings := MakeTLPeerNotifySettings(&PeerNotifySettings{
		ShowPreviews:        ToBool(true),
		Silent:              ToBool(false),
		MuteUntil:           &wrapperspb.Int32Value{Value: 0},
		Sound:               &wrapperspb.StringValue{Value: "default"},
		IosSound:            notificationSoundDefault(),
		AndroidSound:        notificationSoundDefault(),
		OtherSound:          notificationSoundDefault(),
		StoriesMuted:        ToBool(false),
		StoriesHideSender:   ToBool(false),
		StoriesIosSound:     notificationSoundDefault(),
		StoriesAndroidSound: notificationSoundDefault(),
		StoriesOtherSound:   notificationSoundDefault(),
	}).To_PeerNotifySettings()

	return settings
}
