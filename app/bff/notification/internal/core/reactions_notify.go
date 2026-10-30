package core

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
)

// Official clients show contacts for message and story reactions until the
// user changes them. An unset from-field disables that kind of notification.
// Poll-vote notifications stay off until set.
// https://core.telegram.org/constructor/reactionsNotifySettings
// https://core.telegram.org/method/account.getReactionsNotifySettings
const (
	fromContacts  = "contacts"
	fromAll       = "all"
	soundDefault  = "default"
	soundNone     = "none"
	soundLocal    = "local"
	soundRing     = "ringtone"
	soundTitleMax = 64
	soundDataMax  = 128
)

type storedReactions struct {
	Messages string `json:"m,omitempty"`
	Stories  string `json:"s,omitempty"`
	Polls    string `json:"p,omitempty"`
	Sound    string `json:"snd"`
	SoundID  int64  `json:"id,omitempty"`
	Title    string `json:"t,omitempty"`
	Data     string `json:"d,omitempty"`
	Previews bool   `json:"pv"`
}

func defaultReactionsNotifySettings() *mtproto.ReactionsNotifySettings {
	return mtproto.MakeTLReactionsNotifySettings(&mtproto.ReactionsNotifySettings{
		MessagesNotifyFrom: notifyFromContacts(),
		StoriesNotifyFrom:  notifyFromContacts(),
		Sound:              soundDefaultTL(),
		ShowPreviews:       mtproto.BoolTrue,
	}).To_ReactionsNotifySettings()
}

func normalizeReactionsNotifySettings(in *mtproto.ReactionsNotifySettings) *mtproto.ReactionsNotifySettings {
	if in == nil {
		return defaultReactionsNotifySettings()
	}
	sound := normalizeSound(in.GetSound())
	show := mtproto.BoolTrue
	if in.GetShowPreviews() != nil {
		show = mtproto.ToBool(mtproto.FromBool(in.GetShowPreviews()))
	}
	return mtproto.MakeTLReactionsNotifySettings(&mtproto.ReactionsNotifySettings{
		MessagesNotifyFrom:  normalizeFrom(in.GetMessagesNotifyFrom()),
		StoriesNotifyFrom:   normalizeFrom(in.GetStoriesNotifyFrom()),
		PollVotesNotifyFrom: normalizeFrom(in.GetPollVotesNotifyFrom()),
		Sound:               sound,
		ShowPreviews:        show,
	}).To_ReactionsNotifySettings()
}

func encodeReactions(settings *mtproto.ReactionsNotifySettings) (string, error) {
	settings = normalizeReactionsNotifySettings(settings)
	raw, err := json.Marshal(storedReactions{
		Messages: fromToken(settings.GetMessagesNotifyFrom()),
		Stories:  fromToken(settings.GetStoriesNotifyFrom()),
		Polls:    fromToken(settings.GetPollVotesNotifyFrom()),
		Sound:    soundToken(settings.GetSound()),
		SoundID:  settings.GetSound().GetId(),
		Title:    clip(settings.GetSound().GetTitle(), soundTitleMax),
		Data:     clip(settings.GetSound().GetData(), soundDataMax),
		Previews: mtproto.FromBool(settings.GetShowPreviews()),
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeReactions(raw string) (*mtproto.ReactionsNotifySettings, error) {
	var stored storedReactions
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, err
	}
	return mtproto.MakeTLReactionsNotifySettings(&mtproto.ReactionsNotifySettings{
		MessagesNotifyFrom:  fromTokenTL(stored.Messages),
		StoriesNotifyFrom:   fromTokenTL(stored.Stories),
		PollVotesNotifyFrom: fromTokenTL(stored.Polls),
		Sound:               soundFromStored(stored),
		ShowPreviews:        mtproto.ToBool(stored.Previews),
	}).To_ReactionsNotifySettings(), nil
}

func normalizeFrom(v *mtproto.ReactionNotificationsFrom) *mtproto.ReactionNotificationsFrom {
	switch fromToken(v) {
	case fromContacts:
		return notifyFromContacts()
	case fromAll:
		return notifyFromAll()
	default:
		return nil
	}
}

func fromToken(v *mtproto.ReactionNotificationsFrom) string {
	if v == nil {
		return ""
	}
	switch v.PredicateName {
	case mtproto.Predicate_reactionNotificationsFromContacts:
		return fromContacts
	case mtproto.Predicate_reactionNotificationsFromAll:
		return fromAll
	}
	switch v.Constructor {
	case mtproto.CRC32_reactionNotificationsFromContacts:
		return fromContacts
	case mtproto.CRC32_reactionNotificationsFromAll:
		return fromAll
	default:
		return ""
	}
}

func fromTokenTL(token string) *mtproto.ReactionNotificationsFrom {
	switch token {
	case fromContacts:
		return notifyFromContacts()
	case fromAll:
		return notifyFromAll()
	default:
		return nil
	}
}

func notifyFromContacts() *mtproto.ReactionNotificationsFrom {
	return mtproto.MakeTLReactionNotificationsFromContacts(&mtproto.ReactionNotificationsFrom{}).To_ReactionNotificationsFrom()
}

func notifyFromAll() *mtproto.ReactionNotificationsFrom {
	return mtproto.MakeTLReactionNotificationsFromAll(&mtproto.ReactionNotificationsFrom{}).To_ReactionNotificationsFrom()
}

func normalizeSound(v *mtproto.NotificationSound) *mtproto.NotificationSound {
	if v == nil {
		return soundDefaultTL()
	}
	switch soundToken(v) {
	case soundNone:
		return mtproto.MakeTLNotificationSoundNone(&mtproto.NotificationSound{}).To_NotificationSound()
	case soundLocal:
		return mtproto.MakeTLNotificationSoundLocal(&mtproto.NotificationSound{
			Title: clip(v.GetTitle(), soundTitleMax),
			Data:  clip(v.GetData(), soundDataMax),
		}).To_NotificationSound()
	case soundRing:
		return mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{
			Id: v.GetId(),
		}).To_NotificationSound()
	default:
		return soundDefaultTL()
	}
}

func soundToken(v *mtproto.NotificationSound) string {
	if v == nil {
		return soundDefault
	}
	switch v.PredicateName {
	case mtproto.Predicate_notificationSoundNone:
		return soundNone
	case mtproto.Predicate_notificationSoundLocal:
		return soundLocal
	case mtproto.Predicate_notificationSoundRingtone:
		return soundRing
	case mtproto.Predicate_notificationSoundDefault:
		return soundDefault
	}
	switch v.Constructor {
	case mtproto.CRC32_notificationSoundNone:
		return soundNone
	case mtproto.CRC32_notificationSoundLocal:
		return soundLocal
	case mtproto.CRC32_notificationSoundRingtone:
		return soundRing
	case mtproto.CRC32_notificationSoundDefault:
		return soundDefault
	default:
		return soundDefault
	}
}

func soundFromStored(stored storedReactions) *mtproto.NotificationSound {
	switch stored.Sound {
	case soundNone:
		return mtproto.MakeTLNotificationSoundNone(&mtproto.NotificationSound{}).To_NotificationSound()
	case soundLocal:
		return mtproto.MakeTLNotificationSoundLocal(&mtproto.NotificationSound{
			Title: clip(stored.Title, soundTitleMax),
			Data:  clip(stored.Data, soundDataMax),
		}).To_NotificationSound()
	case soundRing:
		return mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{
			Id: stored.SoundID,
		}).To_NotificationSound()
	default:
		return soundDefaultTL()
	}
}

func soundDefaultTL() *mtproto.NotificationSound {
	return mtproto.MakeTLNotificationSoundDefault(&mtproto.NotificationSound{}).To_NotificationSound()
}

func clip(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}
