package appconfig

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/pkg/about"
	"github.com/teamgram/teamgram-server/pkg/announce"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/teamgram/teamgram-server/pkg/crashreport"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
	"github.com/teamgram/teamgram-server/pkg/premium"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
)

// authorizationAutoconfirmPeriod matches
// authsession/internal/dao.AuthorizationAutoconfirmPeriod. A new session
// stops looking unconfirmed in account.getAuthorizations after this many
// seconds. help.getAppConfig publishes the same number.
const authorizationAutoconfirmPeriod = 7 * 24 * 60 * 60

// EnvTranscribeEnabled advertises a speech-to-text trial in help.getAppConfig.
// Default false: transcribe_audio_trial_* is 0, so non-Premium clients hide the
// button. The SenseVoice handler flips this default to true once
// messages.transcribeAudio returns a transcript. Enabled values stay the
// numbers clients already understood (100000 / 300 / 0).
const EnvTranscribeEnabled = premium.EnvTranscribeEnabled

// AppConfig is the help.getAppConfig document official clients read for
// Premium limits and emoji_blocklist. The numbers follow the server policy:
// a feature that non-members may use is advertised at the raised cap for both keys.
func AppConfig() *mtproto.JSONValue {
	return jsonObject(appConfigPairs()...)
}

// AppConfigHash is the hash clients send back on help.getAppConfig.
// It changes when the premium policy, the public link hosts, the help URL,
// the emoji blocklist, a Tellomi feature flag, or the autologin / url-auth
// / whitelist domain lists change.
func AppConfigHash() int32 {
	h := fnv.New32a()
	weekly, duration, cooldown := transcribeTrial()
	ai := 0
	if tellomiai.Advertised() {
		ai = 1
	}
	translate := 0
	if tellomiai.TranslateAdvertised() {
		translate = 1
	}
	compose := 0
	if tellomiai.ComposeAdvertised() {
		compose = 1
	}
	notice := 0
	if announce.Advertised() {
		notice = 1
	}
	fmt.Fprintf(h, "policy=%d brand=%s help=%s reaction=%s blocklist=%s transcribe=%d:%d:%d autoconfirm=%d ai=%d translate=%d compose=%d announce=%d venue=%s crashbot=%s %s %s", premium.AppConfigHash(), brand.LinkPrefix, brand.HelpURL(), catalog.DefaultReactionEmoji, strings.Join(emojiblock.List(), "\x00"), weekly, duration, cooldown, authorizationAutoconfirmPeriod, ai, translate, compose, notice, mtproto.BotVenueName, crashreport.Username(), about.HashFragment(), domainListHashInput())
	if origin, ok := discoverFeedOrigin(); ok {
		fmt.Fprintf(h, " feed=1 %s", origin)
	}
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
	pairs := []*mtproto.JSONObjectValue{
		jsonPair("autologin_domains", domainListValue(autologinDomains())),
		jsonPair("url_auth_domains", domainListValue(urlAuthDomains())),
		jsonPair("whitelisted_domains", domainListValue(whitelistedDomains())),
		jsonPair("premium_link", jsonString(brand.PremiumURL)),
		jsonPair("support_link", jsonString(brand.SupportURL)),
		jsonPair("faq_link", jsonString(brand.HelpURL())),
		jsonPair("privacy_policy_url", jsonString(brand.PrivacyURL)),
		jsonPair("tos_url", jsonString(brand.TermsURL)),
		jsonPair("telegram_antispam", jsonString(brand.AntispamURL)),
		jsonPair("premium_purchase_blocked", jsonBool(premium.Load().PurchaseBlocked)),
		jsonPair("stickers_premium_by_emoji_num", n(premium.CustomEmojiAllowance())),
		jsonPair("reactions_uniq_max", n(11)),
		// Same caps messages.sendMedia enforces for inputMediaTodo.
		jsonPair("todo_items_max", n(mtproto.TodoItemsMax)),
		jsonPair("todo_item_length_max", n(mtproto.TodoItemLengthMax)),
		jsonPair("todo_title_length_max", n(mtproto.TodoTitleLengthMax)),
		jsonPair("reactions_default", jsonString(catalog.DefaultReactionEmoji)),
		jsonPair("emoji_blocklist", emojiBlocklistValue()),
		jsonPair("stories_posting", jsonString("enabled")),
		jsonPair("stories_venue_search_username", jsonString(mtproto.BotVenueName)),
		jsonPair("pm_read_date_expire_period", n(604800)),
		// Same 7 days as dao.AuthorizationAutoconfirmPeriod. Clients that
		// already default to 604800 stay aligned with getAuthorizations.
		jsonPair("authorization_autoconfirm_period", n(authorizationAutoconfirmPeriod)),
	}
	weekly, duration, cooldown := transcribeTrial()
	pairs = append(pairs,
		jsonPair("transcribe_audio_trial_weekly_number", n(weekly)),
		jsonPair("transcribe_audio_trial_duration_max", n(duration)),
		jsonPair("transcribe_audio_trial_cooldown_until", n(cooldown)),
	)
	pairs = append(pairs, hiddenFeaturePairs()...)
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
	pairs = append(pairs, aboutPairs()...)
	if origin, ok := discoverFeedOrigin(); ok {
		pairs = append(pairs,
			jsonPair("tellomi_discover_feed_enabled", jsonBool(true)),
			jsonPair("tellomi_discover_feed_base_url", jsonString(origin)),
		)
	}
	return pairs
}

const (
	// EnvDiscoverFeed publishes the client feed keys. Only the exact value 1
	// does. Credential issuing stays on TELLOMI_DISCOVER.
	EnvDiscoverFeed = "TELLOMI_DISCOVER_FEED"
	// EnvDiscoverFeedURL is the HTTPS origin the client uses for the feed.
	// It is not TELLOMI_DISCOVER_ENGINE_URL.
	EnvDiscoverFeedURL = "TELLOMI_DISCOVER_FEED_BASE_URL"
)

// discoverFeedOrigin is the client feed origin, or false when the keys must
// stay absent. The default (unset, or any value other than 1, or a URL that
// is not an https origin) adds nothing, so the previous config and hash
// stay the same.
func discoverFeedOrigin() (string, bool) {
	if strings.TrimSpace(os.Getenv(EnvDiscoverFeed)) != "1" {
		return "", false
	}
	return httpsOrigin(os.Getenv(EnvDiscoverFeedURL))
}

// httpsOrigin accepts an https URL whose only meaningful parts are the host
// and an optional port. A path, query, fragment, userinfo, empty host, or
// port outside 1..65535 is rejected. The result is https:// plus the host.
func httpsOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n") {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return "", false
	}
	if !strings.EqualFold(u.Scheme, "https") || u.Opaque != "" || u.User != nil {
		return "", false
	}
	if u.Hostname() == "" || u.Host == "" {
		return "", false
	}
	if u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", false
	}
	if p := u.Port(); p != "" {
		n, conv := strconv.Atoi(p)
		if conv != nil || n < 1 || n > 65535 {
			return "", false
		}
	}
	return "https://" + u.Host, true
}

// aboutPairs are Tellomi keys for the privacy switch and the About page.
// Official clients ignore them. String keys are omitted until disclosed.
func aboutPairs() []*mtproto.JSONObjectValue {
	pairs := []*mtproto.JSONObjectValue{
		jsonPair("tellomi_personalized_recommend_default", jsonBool(about.PersonalizedRecommendDefault())),
	}
	if s := about.AlgorithmFilingNumber(); s != "" {
		pairs = append(pairs, jsonPair("tellomi_algorithm_filing_number", jsonString(s)))
	}
	if s := about.AIModelName(); s != "" {
		pairs = append(pairs, jsonPair("tellomi_ai_model_name", jsonString(s)))
	}
	if s := about.AIRegistrationNumber(); s != "" {
		pairs = append(pairs, jsonPair("tellomi_ai_registration_number", jsonString(s)))
	}
	return pairs
}

// transcribeTrial is the (weekly count, max seconds, cooldown unix time)
// triple published as transcribe_audio_trial_*.
func transcribeTrial() (weekly, durationMax, cooldownUntil int) {
	if !premium.TranscribeEnabled() {
		return 0, 0, 0
	}
	return premium.TranscribeTrialWeekly, 300, 0
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// hiddenFeaturePairs are appConfig keys official Android, iOS, and Desktop
// clients read when deciding whether to show a dead Telegram feature.
// Values are the ones that hide the entry. Keys the clients do not read are
// omitted; see the comment on each key.
func hiddenFeaturePairs() []*mtproto.JSONObjectValue {
	off := jsonBool(false)
	on := jsonBool(true)
	return []*mtproto.JSONObjectValue{
		// Stars row and Star UI. Android starsPurchaseAvailable is !starsLocked.
		// iOS areStarsDisabled. Both default to hidden when the key is absent;
		// sending true keeps a client that defaults the other way hidden.
		jsonPair("stars_purchase_blocked", on),
		// Android starsGiftsEnabled defaults to true. iOS defaults to false.
		jsonPair("stars_gifts_enabled", off),
		// "If true, gifts must be disabled." Both clients default to blocked.
		jsonPair("stargifts_blocked", on),
		// Android profile gifts tab. Default true, so it must be sent false.
		jsonPair("enable_gifts_in_profile", off),
		// Gift icons in the attach menu and the text field. Defaults are false.
		jsonPair("premium_gift_attach_menu_icon", off),
		jsonPair("premium_gift_text_field_icon", off),
		// Android starsPaidMessagesAvailable defaults to true.
		jsonPair("stars_paid_messages_available", off),
		// Starting a giveaway. Defaults are false. This is the boosts purchase gate.
		jsonPair("giveaway_gifts_purchase_available", off),
		// "Otherwise, all ad revenue-related UI options should be hidden."
		jsonPair("channel_revenue_withdrawal_enabled", off),
		// Desktop affiliate program gates. Defaults are false.
		jsonPair("starref_program_allowed", off),
		jsonPair("starref_connect_allowed", off),
		// Desktop walletAvailable defaults to true. Android settings and iOS
		// wallet both read this key; release Android and iOS default to false.
		jsonPair("wallet_available", off),
		// Android stores "enabled" when the key has never been seen. iOS treats
		// a missing key as disabled. Send disabled so both stay off.
		jsonPair("translations_manual_enabled", jsonString(translationSwitch(tellomiai.LocalReady()))),
		jsonPair("translations_auto_enabled", jsonString(translationSwitch(tellomiai.AutoReady()))),
		// Tellomi-only. Clients will read these; official builds ignore them.
		jsonPair("tellomi_ai_compose_enabled", jsonBool(tellomiai.ComposeAdvertised())),
		jsonPair("tellomi_ai_enabled", jsonBool(tellomiai.Advertised())),
		jsonPair("tellomi_translate_enabled", jsonBool(tellomiai.TranslateAdvertised())),
		jsonPair("tellomi_announce_enabled", jsonBool(announce.Advertised())),
		// iOS and Android look this username up and send crash logs there.
		jsonPair("tellomi_crash_report_bot", jsonString(crashreport.Username())),
	}
}

func emojiBlocklistValue() *mtproto.JSONValue {
	list := emojiblock.List()
	vals := make([]*mtproto.JSONValue, len(list))
	for i, emo := range list {
		vals[i] = jsonString(emo)
	}
	return jsonArray(vals...)
}

func jsonBool(v bool) *mtproto.JSONValue {
	return mtproto.MakeTLJsonBool(&mtproto.JSONValue{Value_BOOL: mtproto.ToBool(v)}).To_JSONValue()
}

func jsonNumber(v float64) *mtproto.JSONValue {
	return mtproto.MakeTLJsonNumber(&mtproto.JSONValue{Value_FLOAT64: v}).To_JSONValue()
}

func translationSwitch(on bool) string {
	if on {
		return "enabled"
	}
	return "disabled"
}

// Short-link root hosts (tell.cc) and the official site host (tellomi.cn)
// must not appear in autologin_domains or url_auth_domains. Official
// Android clients treat url_auth_domains as "open in an external browser"
// and would otherwise force tell.cc links out of the app. Web SSO, if
// needed later, should use a dedicated host.
func autologinDomains() []string { return nil }
func urlAuthDomains() []string   { return nil }

func whitelistedDomains() []string {
	return []string{brand.ShortHost, brand.SiteHost}
}

func domainListValue(hosts []string) *mtproto.JSONValue {
	vals := make([]*mtproto.JSONValue, 0, len(hosts))
	for _, host := range hosts {
		vals = append(vals, jsonString(host))
	}
	return jsonArray(vals...)
}

func domainListHashInput() string {
	return "autologin=" + strings.Join(autologinDomains(), ",") +
		" urlauth=" + strings.Join(urlAuthDomains(), ",") +
		" whitelist=" + strings.Join(whitelistedDomains(), ",")
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
