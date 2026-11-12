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
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	DefaultPeerNotifySettings = MakeDefaultPeerNotifySettings()
)

func MakePeerNotifySettings(settings *InputPeerNotifySettings) (*PeerNotifySettings, error) {
	if settings == nil {
		settings = &InputPeerNotifySettings{}
	}
	notifySettings := MakeTLPeerNotifySettings(&PeerNotifySettings{
		ShowPreviews:      settings.GetShowPreviews(),
		Silent:            settings.GetSilent(),
		MuteUntil:         settings.GetMuteUntil(),
		StoriesMuted:      settings.GetStoriesMuted(),
		StoriesHideSender: settings.GetStoriesHideSender(),
	}).To_PeerNotifySettings()

	// inputPeerNotifySettings has one sound. peerNotifySettings has one per
	// platform. The same value is stored for ios, android and other.
	if sound := settings.GetSound_FLAGNOTIFICATIONSOUND(); sound != nil {
		notifySettings.IosSound = sound
		notifySettings.AndroidSound = sound
		notifySettings.OtherSound = sound
		notifySettings.Sound = &wrapperspb.StringValue{Value: LegacySound(sound)}
	} else if legacy := settings.GetSound_FLAGSTRING(); legacy != nil {
		notifySettings.Sound = legacy
		if parsed := SoundFromLegacy(legacy.GetValue()); parsed != nil {
			notifySettings.IosSound = parsed
			notifySettings.AndroidSound = parsed
			notifySettings.OtherSound = parsed
		}
	}
	if sound := settings.GetStoriesSound(); sound != nil {
		notifySettings.StoriesIosSound = sound
		notifySettings.StoriesAndroidSound = sound
		notifySettings.StoriesOtherSound = sound
	}
	return notifySettings, nil
}

// SoundFromLegacy reads a sound stored before NotificationSound existed.
// "default" and a positive decimal id are the values already written in
// user_notify_settings.sound. "-1" and "" mean the field was never set.
func SoundFromLegacy(s string) *NotificationSound {
	switch s {
	case "", "-1":
		return nil
	case "default":
		return notificationSoundDefault()
	case "none", "0":
		return notificationSoundNone()
	default:
		if strings.HasPrefix(s, "local:") {
			title := strings.TrimPrefix(s, "local:")
			return MakeTLNotificationSoundLocal(&NotificationSound{Title: title}).To_NotificationSound()
		}
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			return notificationSoundDefault()
		}
		return MakeTLNotificationSoundRingtone(&NotificationSound{Id: id}).To_NotificationSound()
	}
}

// LegacySound is the varchar stored for push and for older clients.
func LegacySound(sound *NotificationSound) string {
	if sound == nil {
		return "-1"
	}
	switch sound.GetPredicateName() {
	case Predicate_notificationSoundNone:
		return "none"
	case Predicate_notificationSoundRingtone:
		return strconv.FormatInt(sound.GetId(), 10)
	case Predicate_notificationSoundLocal:
		title := sound.GetTitle()
		if len(title) > 240 {
			title = title[:240]
		}
		return "local:" + title
	default:
		return "default"
	}
}

func notificationSoundDefault() *NotificationSound {
	return MakeTLNotificationSoundDefault(&NotificationSound{}).To_NotificationSound()
}

func notificationSoundNone() *NotificationSound {
	return MakeTLNotificationSoundNone(&NotificationSound{}).To_NotificationSound()
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
	sound := notificationSoundDefault()
	// silent is false: a new chat notifies. The comment above this function
	// already said boolFalse; the previous value was the other way around.
	return MakeTLPeerNotifySettings(&PeerNotifySettings{
		ShowPreviews: ToBool(true),
		Silent:       ToBool(false),
		MuteUntil:    &wrapperspb.Int32Value{Value: 0},
		Sound:        &wrapperspb.StringValue{Value: "default"},
		IosSound:     sound,
		AndroidSound: sound,
		OtherSound:   sound,
	}).To_PeerNotifySettings()
}
