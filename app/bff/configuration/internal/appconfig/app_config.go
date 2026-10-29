package appconfig

import (
	"fmt"
	"hash/fnv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

// AppConfig is the help.getAppConfig document official clients read for
// Premium limits. The numbers follow the server policy: a feature that
// non-members may use is advertised at the raised cap for both keys.
func AppConfig() *mtproto.JSONValue {
	return jsonObject(appConfigPairs()...)
}

// AppConfigHash is the hash clients send back on help.getAppConfig.
// It changes when the premium policy or the public link hosts change.
func AppConfigHash() int32 {
	h := fnv.New32a()
	fmt.Fprintf(h, "policy=%d brand=%s reaction=%s", premium.AppConfigHash(), brand.LinkPrefix, catalog.DefaultReactionEmoji)
	sum := int32(h.Sum32() & 0x7fffffff)
	if sum == 0 {
		return 1
	}
	return sum
}

func appConfigPairs() []*mtproto.JSONObjectValue {
	n := func(v int) *mtproto.JSONValue {
		return jsonNumber(float64(v))
	}
	pair := func(key string, feature premium.Feature) []*mtproto.JSONObjectValue {
		def, prem := premium.Advertised(feature)
		return []*mtproto.JSONObjectValue{
			jsonPair(key+"_default", n(def)),
			jsonPair(key+"_premium", n(prem)),
		}
	}
	chatsDef, chatsPrem := premium.DialogFilterChatLimit()
	domains := jsonArray(jsonString(brand.ShortHost), jsonString(brand.SiteHost))
	pairs := []*mtproto.JSONObjectValue{
		jsonPair("autologin_domains", domains),
		jsonPair("url_auth_domains", domains),
		jsonPair("whitelisted_domains", domains),
		jsonPair("premium_link", jsonString(brand.PremiumURL)),
		jsonPair("support_link", jsonString(brand.SupportURL)),
		jsonPair("faq_link", jsonString(brand.FAQURL)),
		jsonPair("privacy_policy_url", jsonString(brand.PrivacyURL)),
		jsonPair("tos_url", jsonString(brand.TermsURL)),
		jsonPair("telegram_antispam", jsonString(brand.AntispamURL)),
		jsonPair("premium_purchase_blocked", jsonBool(premium.Load().PurchaseBlocked)),
		jsonPair("stickers_premium_by_emoji_num", n(premium.CustomEmojiAllowance())),
		jsonPair("reactions_uniq_max", n(11)),
		jsonPair("reactions_default", jsonString(catalog.DefaultReactionEmoji)),
		jsonPair("transcribe_audio_trial_weekly_number", n(100000)),
		jsonPair("transcribe_audio_trial_duration_max", n(300)),
		jsonPair("transcribe_audio_trial_cooldown_until", n(0)),
		jsonPair("stories_posting", jsonString("enabled")),
		jsonPair("pm_read_date_expire_period", n(604800)),
	}
	pairs = append(pairs, pair("about_length_limit", premium.FeatureAbout)...)
	pairs = append(pairs, pair("caption_length_limit", premium.FeatureCaption)...)
	pairs = append(pairs, pair("dialogs_pinned_limit", premium.FeaturePinnedChats)...)
	pairs = append(pairs, pair("dialogs_folder_pinned_limit", premium.FeatureFolderPins)...)
	pairs = append(pairs, pair("dialog_filters_limit", premium.FeatureDialogFilters)...)
	pairs = append(pairs, jsonPair("dialog_filters_chats_limit_default", n(chatsDef)), jsonPair("dialog_filters_chats_limit_premium", n(chatsPrem)))
	pairs = append(pairs, pair("saved_gifs_limit", premium.FeatureSavedGifs)...)
	pairs = append(pairs, pair("stickers_faved_limit", premium.FeatureFavedStickers)...)
	pairs = append(pairs, pair("channels_limit", premium.FeatureChannels)...)
	pairs = append(pairs, pair("channels_public_limit", premium.FeaturePublicChannels)...)
	pairs = append(pairs, pair("reactions_user_max", premium.FeatureReactions)...)
	pairs = append(pairs, pair("upload_max_fileparts", premium.FeatureUpload)...)
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

func jsonArray(vals ...*mtproto.JSONValue) *mtproto.JSONValue {
	return mtproto.MakeTLJsonArray(&mtproto.JSONValue{Value_VECTORJSONVALUE: vals}).To_JSONValue()
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
