package appconfig

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

// AppConfig is the help.getAppConfig document official clients read for
// Premium limits. Default and Premium values are the same raised limits,
// and premium_purchase_blocked hides the store purchase UI.
func AppConfig() *mtproto.JSONValue {
	return jsonObject(appConfigPairs()...)
}

// AppConfigHash is the hash clients send back on help.getAppConfig.
func AppConfigHash() int32 {
	return premium.AppConfigHash
}

func appConfigPairs() []*mtproto.JSONObjectValue {
	n := func(v int) *mtproto.JSONValue {
		return jsonNumber(float64(v))
	}
	same := func(key string, v int) []*mtproto.JSONObjectValue {
		return []*mtproto.JSONObjectValue{
			jsonPair(key+"_default", n(v)),
			jsonPair(key+"_premium", n(v)),
		}
	}
	pairs := []*mtproto.JSONObjectValue{
		jsonPair("premium_purchase_blocked", jsonBool(true)),
		jsonPair("stickers_premium_by_emoji_num", n(premium.CustomEmojiPerMessage)),
		jsonPair("reactions_uniq_max", n(11)),
		jsonPair("transcribe_audio_trial_weekly_number", n(100000)),
		jsonPair("transcribe_audio_trial_duration_max", n(300)),
		jsonPair("transcribe_audio_trial_cooldown_until", n(0)),
		jsonPair("stories_posting", jsonString("enabled")),
		jsonPair("pm_read_date_expire_period", n(604800)),
	}
	pairs = append(pairs, same("about_length_limit", premium.AboutLength)...)
	pairs = append(pairs, same("caption_length_limit", premium.CaptionLength)...)
	pairs = append(pairs, same("dialogs_pinned_limit", premium.PinnedDialogs)...)
	pairs = append(pairs, same("dialogs_folder_pinned_limit", premium.FolderPinnedDialogs)...)
	pairs = append(pairs, same("dialog_filters_limit", premium.DialogFilters)...)
	pairs = append(pairs, same("dialog_filters_chats_limit", premium.DialogFilterChats)...)
	pairs = append(pairs, same("saved_gifs_limit", premium.SavedGifs)...)
	pairs = append(pairs, same("stickers_faved_limit", premium.FavedStickers)...)
	pairs = append(pairs, same("channels_limit", premium.Channels)...)
	pairs = append(pairs, same("channels_public_limit", premium.PublicChannels)...)
	pairs = append(pairs, same("reactions_user_max", premium.ReactionsUserMax)...)
	pairs = append(pairs, same("upload_max_fileparts", premium.UploadMaxFileParts)...)
	return pairs
}

func jsonBool(v bool) *mtproto.JSONValue {
	return mtproto.MakeTLJsonBool(&mtproto.JSONValue{Value_BOOL: mtproto.ToBool(v)}).To_JSONValue()
}

func jsonNumber(v float64) *mtproto.JSONValue {
	return mtproto.MakeTLJsonNumber(&mtproto.JSONValue{Value_FLOAT64: v}).To_JSONValue()
}

func jsonString(v string) *mtproto.JSONValue {
	return mtproto.MakeTLJsonString(&mtproto.JSONValue{Value_STRING: v}).To_JSONValue()
}

func jsonObject(pairs ...*mtproto.JSONObjectValue) *mtproto.JSONValue {
	return mtproto.MakeTLJsonObject(&mtproto.JSONValue{
		Value_VECTORJSONOBJECTVALUE: pairs,
	}).To_JSONValue()
}

func jsonPair(key string, value *mtproto.JSONValue) *mtproto.JSONObjectValue {
	return mtproto.MakeTLJsonObjectValue(&mtproto.JSONObjectValue{
		Key:   key,
		Value: value,
	}).To_JSONObjectValue()
}
