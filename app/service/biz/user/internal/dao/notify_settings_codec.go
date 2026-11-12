package dao

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/zeromicro/go-zero/core/jsonx"
)

// soundJSON is one NotificationSound. Predicate is the TL name
// (notificationSoundDefault, notificationSoundNone, notificationSoundLocal,
// notificationSoundRingtone).
type soundJSON struct {
	Predicate string `json:"predicate,omitempty"`
	Title     string `json:"title,omitempty"`
	Data      string `json:"data,omitempty"`
	ID        int64  `json:"id,omitempty"`
}

func soundToJSON(s *mtproto.NotificationSound) *soundJSON {
	if s == nil || s.GetPredicateName() == "" {
		return nil
	}
	return &soundJSON{
		Predicate: s.GetPredicateName(),
		Title:     s.GetTitle(),
		Data:      s.GetData(),
		ID:        s.GetId(),
	}
}

func soundFromJSON(s *soundJSON) *mtproto.NotificationSound {
	if s == nil || s.Predicate == "" {
		return nil
	}
	switch s.Predicate {
	case mtproto.Predicate_notificationSoundNone:
		return mtproto.MakeTLNotificationSoundNone(&mtproto.NotificationSound{}).To_NotificationSound()
	case mtproto.Predicate_notificationSoundLocal:
		return mtproto.MakeTLNotificationSoundLocal(&mtproto.NotificationSound{Title: s.Title, Data: s.Data}).To_NotificationSound()
	case mtproto.Predicate_notificationSoundRingtone:
		return mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{Id: s.ID}).To_NotificationSound()
	default:
		return mtproto.MakeTLNotificationSoundDefault(&mtproto.NotificationSound{}).To_NotificationSound()
	}
}

func boolJSON(v *mtproto.Bool) *bool {
	if v == nil {
		return nil
	}
	b := mtproto.FromBool(v)
	return &b
}

func boolFromJSON(v *bool) *mtproto.Bool {
	if v == nil {
		return nil
	}
	return mtproto.ToBool(*v)
}

// notifyExtra is the part of peerNotifySettings that does not fit in the
// original four columns: platform sounds and stories notification fields.
type notifyExtra struct {
	Ios            *soundJSON `json:"ios_sound,omitempty"`
	Android        *soundJSON `json:"android_sound,omitempty"`
	Other          *soundJSON `json:"other_sound,omitempty"`
	StoriesMuted   *bool      `json:"stories_muted,omitempty"`
	StoriesHide    *bool      `json:"stories_hide_sender,omitempty"`
	StoriesIos     *soundJSON `json:"stories_ios_sound,omitempty"`
	StoriesAndroid *soundJSON `json:"stories_android_sound,omitempty"`
	StoriesOther   *soundJSON `json:"stories_other_sound,omitempty"`
}

func (e notifyExtra) empty() bool {
	return e.Ios == nil && e.Android == nil && e.Other == nil &&
		e.StoriesMuted == nil && e.StoriesHide == nil &&
		e.StoriesIos == nil && e.StoriesAndroid == nil && e.StoriesOther == nil
}

func extraFromSettings(s *mtproto.PeerNotifySettings) notifyExtra {
	if s == nil {
		return notifyExtra{}
	}
	return notifyExtra{
		Ios:            soundToJSON(s.IosSound),
		Android:        soundToJSON(s.AndroidSound),
		Other:          soundToJSON(s.OtherSound),
		StoriesMuted:   boolJSON(s.StoriesMuted),
		StoriesHide:    boolJSON(s.StoriesHideSender),
		StoriesIos:     soundToJSON(s.StoriesIosSound),
		StoriesAndroid: soundToJSON(s.StoriesAndroidSound),
		StoriesOther:   soundToJSON(s.StoriesOtherSound),
	}
}

func encodeNotifyExtra(s *mtproto.PeerNotifySettings) (string, error) {
	extra := extraFromSettings(s)
	if extra.empty() {
		return "", nil
	}
	raw, err := jsonx.Marshal(extra)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func applyNotifyExtra(s *mtproto.PeerNotifySettings, raw string) {
	if s == nil || raw == "" {
		return
	}
	var extra notifyExtra
	if err := jsonx.UnmarshalFromString(raw, &extra); err != nil {
		return
	}
	if sound := soundFromJSON(extra.Ios); sound != nil {
		s.IosSound = sound
	}
	if sound := soundFromJSON(extra.Android); sound != nil {
		s.AndroidSound = sound
	}
	if sound := soundFromJSON(extra.Other); sound != nil {
		s.OtherSound = sound
	}
	if b := boolFromJSON(extra.StoriesMuted); b != nil {
		s.StoriesMuted = b
	}
	if b := boolFromJSON(extra.StoriesHide); b != nil {
		s.StoriesHideSender = b
	}
	if sound := soundFromJSON(extra.StoriesIos); sound != nil {
		s.StoriesIosSound = sound
	}
	if sound := soundFromJSON(extra.StoriesAndroid); sound != nil {
		s.StoriesAndroidSound = sound
	}
	if sound := soundFromJSON(extra.StoriesOther); sound != nil {
		s.StoriesOtherSound = sound
	}
}

// PeerNotifyFromDO rebuilds the TL object from one stored row.
func PeerNotifyFromDO(do *dataobject.UserNotifySettingsDO) *mtproto.PeerNotifySettings {
	settings := mtproto.MakeTLPeerNotifySettings(nil).To_PeerNotifySettings()
	if do == nil {
		return settings
	}
	setPeerNotifySettingsByDO(settings, do)
	applyNotifyExtra(settings, do.SettingsJSON)
	if settings.IosSound == nil && settings.AndroidSound == nil && settings.OtherSound == nil && do.Sound != "-1" {
		if sound := mtproto.SoundFromLegacy(do.Sound); sound != nil {
			settings.IosSound = sound
			settings.AndroidSound = sound
			settings.OtherSound = sound
		}
	}
	return settings
}

// SettingsFromRow is the getNotifySettings result for a row, or the default
// when the user has never saved one. A missing per-chat row stays empty so
// the client inherits the users/chats/channels scope. The global scopes
// themselves use the documented defaults, including silent = false.
func SettingsFromRow(peerType int32, do *dataobject.UserNotifySettingsDO) *mtproto.PeerNotifySettings {
	if do == nil {
		switch peerType {
		case mtproto.PEER_USERS, mtproto.PEER_CHATS, mtproto.PEER_BROADCASTS:
			return mtproto.MakeDefaultPeerNotifySettings()
		default:
			return mtproto.MakeTLPeerNotifySettings(nil).To_PeerNotifySettings()
		}
	}
	return PeerNotifyFromDO(do)
}

// MergePeerNotify keeps fields the input did not set.
func MergePeerNotify(base, patch *mtproto.PeerNotifySettings) *mtproto.PeerNotifySettings {
	if base == nil {
		base = mtproto.MakeTLPeerNotifySettings(nil).To_PeerNotifySettings()
	}
	out := mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		ShowPreviews:        base.ShowPreviews,
		Silent:              base.Silent,
		MuteUntil:           base.MuteUntil,
		Sound:               base.Sound,
		IosSound:            base.IosSound,
		AndroidSound:        base.AndroidSound,
		OtherSound:          base.OtherSound,
		StoriesMuted:        base.StoriesMuted,
		StoriesHideSender:   base.StoriesHideSender,
		StoriesIosSound:     base.StoriesIosSound,
		StoriesAndroidSound: base.StoriesAndroidSound,
		StoriesOtherSound:   base.StoriesOtherSound,
	}).To_PeerNotifySettings()
	if patch == nil {
		return out
	}
	if patch.ShowPreviews != nil {
		out.ShowPreviews = patch.ShowPreviews
	}
	if patch.Silent != nil {
		out.Silent = patch.Silent
	}
	if patch.MuteUntil != nil {
		out.MuteUntil = patch.MuteUntil
	}
	if patch.Sound != nil {
		out.Sound = patch.Sound
	}
	if patch.IosSound != nil {
		out.IosSound = patch.IosSound
	}
	if patch.AndroidSound != nil {
		out.AndroidSound = patch.AndroidSound
	}
	if patch.OtherSound != nil {
		out.OtherSound = patch.OtherSound
	}
	if patch.StoriesMuted != nil {
		out.StoriesMuted = patch.StoriesMuted
	}
	if patch.StoriesHideSender != nil {
		out.StoriesHideSender = patch.StoriesHideSender
	}
	if patch.StoriesIosSound != nil {
		out.StoriesIosSound = patch.StoriesIosSound
	}
	if patch.StoriesAndroidSound != nil {
		out.StoriesAndroidSound = patch.StoriesAndroidSound
	}
	if patch.StoriesOtherSound != nil {
		out.StoriesOtherSound = patch.StoriesOtherSound
	}
	return out
}
