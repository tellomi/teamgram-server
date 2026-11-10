package appconfig

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/pkg/about"
	"github.com/teamgram/teamgram-server/pkg/announce"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/teamgram/teamgram-server/pkg/crashreport"
	"github.com/teamgram/teamgram-server/pkg/discoverauth"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
	"github.com/teamgram/teamgram-server/pkg/premium"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
)

func TestAppConfigPremiumPolicy(t *testing.T) {
	t.Setenv(tellomiai.EnvTranslate, "")
	t.Setenv(tellomiai.EnvCompose, "")
	t.Setenv(tellomiai.EnvAPIKey, "")
	t.Cleanup(func() { emojiblock.Reload() })
	t.Setenv(emojiblock.EnvExtra, "")
	t.Setenv(emojiblock.EnvFile, "")
	emojiblock.Reload()
	t.Setenv(premium.EnvEveryone, "true")
	t.Setenv(premium.EnvFreeFeatures, "")
	t.Setenv(premium.EnvPurchaseBlocked, "true")
	t.Setenv(EnvTranscribeEnabled, "")
	t.Setenv(about.EnvEnabled, "")
	t.Setenv(about.EnvAlgorithmFiling, "")
	t.Setenv(about.EnvAIModel, "")
	t.Setenv(about.EnvAIRegistration, "")
	t.Setenv(EnvDiscoverFeed, "")
	t.Setenv(EnvDiscoverFeedURL, "")
	premium.Reload()
	t.Cleanup(func() { premium.Reload() })

	raw := mtproto.FromJSONValue(AppConfig())
	obj, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("app config is %T", raw)
	}
	if obj["premium_purchase_blocked"] != true {
		t.Fatalf("premium_purchase_blocked = %#v", obj["premium_purchase_blocked"])
	}
	want := map[string]float64{
		"about_length_limit_default":          float64(premium.AboutLength),
		"about_length_limit_premium":          float64(premium.AboutLength),
		"caption_length_limit_default":        float64(premium.CaptionLength),
		"caption_length_limit_premium":        float64(premium.CaptionLength),
		"dialogs_pinned_limit_default":        float64(premium.PinnedDialogs),
		"dialogs_pinned_limit_premium":        float64(premium.PinnedDialogs),
		"dialogs_folder_pinned_limit_default": float64(premium.FolderPinnedDialogs),
		"dialog_filters_limit_default":        float64(premium.DialogFilters),
		"dialog_filters_limit_premium":        float64(premium.DialogFilters),
		"stickers_faved_limit_premium":        float64(premium.FavedStickers),
		"reactions_user_max_default":          float64(premium.ReactionsUserMax),
		"upload_max_fileparts_premium":        float64(premium.UploadMaxFileParts),
		"stickers_premium_by_emoji_num":       float64(premium.CustomEmojiAllowance()),
	}
	for key, value := range want {
		got, ok := obj[key].(float64)
		if !ok || got != value {
			t.Fatalf("%s = %#v, want %v", key, obj[key], value)
		}
	}
	if AppConfigHash() == 0 {
		t.Fatal("app config hash is zero")
	}
	if obj["authorization_autoconfirm_period"] != float64(authorizationAutoconfirmPeriod) || authorizationAutoconfirmPeriod != 604800 {
		t.Fatalf("authorization_autoconfirm_period = %#v", obj["authorization_autoconfirm_period"])
	}
	if obj["todo_items_max"] != float64(mtproto.TodoItemsMax) || obj["todo_item_length_max"] != float64(mtproto.TodoItemLengthMax) || obj["todo_title_length_max"] != float64(mtproto.TodoTitleLengthMax) {
		t.Fatalf("todo limits %#v %#v %#v", obj["todo_items_max"], obj["todo_item_length_max"], obj["todo_title_length_max"])
	}
	if obj["reactions_default"] != catalog.DefaultReactionEmoji || catalog.DefaultReactionEmoji != "👍" {
		t.Fatalf("reactions_default = %#v", obj["reactions_default"])
	}
	for _, key := range []string{"premium_link", "support_link", "faq_link", "privacy_policy_url", "tos_url", "telegram_antispam"} {
		s, _ := obj[key].(string)
		if s == "" || strings.Contains(s, "t.me") || strings.Contains(s, "telegram.org") || strings.Contains(s, "telegram.me") {
			t.Fatalf("%s = %q", key, s)
		}
	}
	domains, _ := obj["autologin_domains"].([]any)
	if len(domains) != 2 || domains[0] != "tell.cc" || domains[1] != "tellomi.cn" {
		t.Fatalf("autologin_domains = %#v", domains)
	}
	blocklist, _ := obj["emoji_blocklist"].([]any)
	if len(blocklist) != 1 || blocklist[0] != emojiblock.TaiwanFlag {
		t.Fatalf("emoji_blocklist = %#v", blocklist)
	}
	assertHiddenFeatures(t, obj)
	if _, ok := obj["tellomi_discover_feed_enabled"]; ok || obj["tellomi_discover_feed_base_url"] != nil {
		t.Fatalf("default feed keys %#v %#v", obj["tellomi_discover_feed_enabled"], obj["tellomi_discover_feed_base_url"])
	}
	if obj["transcribe_audio_trial_weekly_number"] != float64(0) ||
		obj["transcribe_audio_trial_duration_max"] != float64(0) ||
		obj["transcribe_audio_trial_cooldown_until"] != float64(0) {
		t.Fatalf("transcribe trial = %v %v %v", obj["transcribe_audio_trial_weekly_number"], obj["transcribe_audio_trial_duration_max"], obj["transcribe_audio_trial_cooldown_until"])
	}
}

func TestAppConfigTranscribeSwitch(t *testing.T) {
	t.Setenv(EnvTranscribeEnabled, "")
	off := AppConfigHash()
	raw := mtproto.FromJSONValue(AppConfig())
	obj := raw.(map[string]any)
	if obj["transcribe_audio_trial_weekly_number"] != float64(0) {
		t.Fatalf("default weekly = %#v", obj["transcribe_audio_trial_weekly_number"])
	}

	t.Setenv(EnvTranscribeEnabled, "true")
	raw = mtproto.FromJSONValue(AppConfig())
	obj = raw.(map[string]any)
	if obj["transcribe_audio_trial_weekly_number"] != float64(100000) ||
		obj["transcribe_audio_trial_duration_max"] != float64(300) ||
		obj["transcribe_audio_trial_cooldown_until"] != float64(0) {
		t.Fatalf("enabled trial = %v %v %v", obj["transcribe_audio_trial_weekly_number"], obj["transcribe_audio_trial_duration_max"], obj["transcribe_audio_trial_cooldown_until"])
	}
	if AppConfigHash() == off {
		t.Fatal("transcribe switch did not change the app config hash")
	}
}

func assertHiddenFeatures(t *testing.T, obj map[string]any) {
	t.Helper()
	bools := map[string]bool{
		"premium_purchase_blocked":               true,
		"stars_purchase_blocked":                 true,
		"stars_gifts_enabled":                    false,
		"stargifts_blocked":                      true,
		"enable_gifts_in_profile":                false,
		"premium_gift_attach_menu_icon":          false,
		"premium_gift_text_field_icon":           false,
		"stars_paid_messages_available":          false,
		"giveaway_gifts_purchase_available":      false,
		"channel_revenue_withdrawal_enabled":     false,
		"starref_program_allowed":                false,
		"starref_connect_allowed":                false,
		"wallet_available":                       false,
		"tellomi_ai_compose_enabled":             false,
		"tellomi_ai_enabled":                     false,
		"tellomi_translate_enabled":              false,
		"tellomi_announce_enabled":               false,
		"tellomi_personalized_recommend_default": false,
	}
	for key, want := range bools {
		if obj[key] != want {
			t.Fatalf("%s = %#v, want %v", key, obj[key], want)
		}
	}
	for _, key := range []string{"translations_manual_enabled", "translations_auto_enabled"} {
		if obj[key] != "disabled" {
			t.Fatalf("%s = %#v", key, obj[key])
		}
	}
}

func TestDiscoverFeedConfig(t *testing.T) {
	t.Setenv(discoverauth.EnvEnabled, "")
	t.Setenv(EnvDiscoverFeed, "")
	t.Setenv(EnvDiscoverFeedURL, "")
	base := AppConfigHash()
	if _, ok := configMap(t)["tellomi_discover_feed_enabled"]; ok {
		t.Fatal("unset published a feed key")
	}

	for _, url := range []string{
		"",
		"   ",
		"http://10.0.2.2:8443",
		"https://user:secret@10.0.2.2:8443",
		"https://10.0.2.2:8443/v1",
		"https://10.0.2.2:8443/",
		"https://10.0.2.2:8443?x=1",
		"https://10.0.2.2:8443#frag",
		"https://10.0.2.2:0",
		"https://10.0.2.2:65536",
		"https://10.0.2.2:abc",
		"https://",
		"https:///feed",
	} {
		t.Setenv(EnvDiscoverFeed, "1")
		t.Setenv(EnvDiscoverFeedURL, url)
		obj := configMap(t)
		if _, ok := obj["tellomi_discover_feed_enabled"]; ok || obj["tellomi_discover_feed_base_url"] != nil {
			t.Fatalf("%q published %#v", url, obj["tellomi_discover_feed_base_url"])
		}
		if AppConfigHash() != base {
			t.Fatalf("%q changed the default hash", url)
		}
	}

	t.Setenv(EnvDiscoverFeed, "true")
	t.Setenv(EnvDiscoverFeedURL, "https://10.0.2.2:8443")
	if _, ok := configMap(t)["tellomi_discover_feed_enabled"]; ok || AppConfigHash() != base {
		t.Fatal("a non-1 switch published the feed")
	}

	t.Setenv(EnvDiscoverFeed, "1")
	t.Setenv(EnvDiscoverFeedURL, " https://10.0.2.2:8443 ")
	obj := configMap(t)
	if obj["tellomi_discover_feed_enabled"] != true || obj["tellomi_discover_feed_base_url"] != "https://10.0.2.2:8443" {
		t.Fatalf("enabled %#v %#v", obj["tellomi_discover_feed_enabled"], obj["tellomi_discover_feed_base_url"])
	}
	on := AppConfigHash()
	if on == base {
		t.Fatal("a valid feed did not change the hash")
	}
	if discoverauth.Enabled() {
		t.Fatal("the feed switch issued credentials")
	}

	t.Setenv(EnvDiscoverFeedURL, "https://[::1]:8443")
	obj = configMap(t)
	if obj["tellomi_discover_feed_base_url"] != "https://[::1]:8443" {
		t.Fatalf("ipv6 %#v", obj["tellomi_discover_feed_base_url"])
	}
	if AppConfigHash() == on {
		t.Fatal("a different origin kept the same hash")
	}

	t.Setenv(EnvDiscoverFeed, "")
	t.Setenv(EnvDiscoverFeedURL, "")
	if AppConfigHash() != base {
		t.Fatal("clearing the feed did not restore the default hash")
	}
	if _, ok := configMap(t)["tellomi_discover_feed_enabled"]; ok {
		t.Fatal("cleared feed still published")
	}
}

func configMap(t *testing.T) map[string]any {
	t.Helper()
	raw := mtproto.FromJSONValue(AppConfig())
	obj, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("app config is %T", raw)
	}
	return obj
}

func TestTranscribeTrialFollowsEnv(t *testing.T) {
	t.Setenv(premium.EnvTranscribeEnabled, "")
	if premium.TranscribeEnabled() {
		t.Fatal("transcription is enabled when TELLOMI_TRANSCRIBE_ENABLED is unset")
	}
	raw := mtproto.FromJSONValue(AppConfig())
	obj, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("app config is %T", raw)
	}
	if obj["transcribe_audio_trial_weekly_number"] != float64(0) {
		t.Fatalf("weekly = %#v", obj["transcribe_audio_trial_weekly_number"])
	}
	if obj["transcribe_audio_trial_duration_max"] != float64(0) || obj["transcribe_audio_trial_cooldown_until"] != float64(0) {
		t.Fatalf("duration/cooldown = %#v %#v", obj["transcribe_audio_trial_duration_max"], obj["transcribe_audio_trial_cooldown_until"])
	}
	off := AppConfigHash()
	t.Setenv(premium.EnvTranscribeEnabled, "true")
	if !premium.TranscribeEnabled() {
		t.Fatal("true did not enable transcription")
	}
	if AppConfigHash() == off {
		t.Fatal("app config hash ignored TELLOMI_TRANSCRIBE_ENABLED")
	}
	raw = mtproto.FromJSONValue(AppConfig())
	obj = raw.(map[string]any)
	if obj["transcribe_audio_trial_weekly_number"] != float64(premium.TranscribeTrialWeekly) {
		t.Fatalf("weekly = %#v", obj["transcribe_audio_trial_weekly_number"])
	}
}

func TestAppConfigEmojiBlocklistExtendsHash(t *testing.T) {
	t.Cleanup(func() { emojiblock.Reload() })
	t.Setenv(emojiblock.EnvExtra, "")
	t.Setenv(emojiblock.EnvFile, "")
	emojiblock.Reload()
	base := AppConfigHash()

	t.Setenv(emojiblock.EnvExtra, "1F44D")
	emojiblock.Reload()
	if AppConfigHash() == base {
		t.Fatal("app config hash ignored the emoji blocklist")
	}
	raw := mtproto.FromJSONValue(AppConfig())
	obj, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("app config is %T", raw)
	}
	blocklist, _ := obj["emoji_blocklist"].([]any)
	if len(blocklist) != 2 || blocklist[0] != emojiblock.TaiwanFlag || blocklist[1] != "\U0001F44D" {
		t.Fatalf("emoji_blocklist = %#v", blocklist)
	}
}

func TestTellomiAIFlagDefaultsOff(t *testing.T) {
	t.Setenv(tellomiai.EnvEnabled, "")
	t.Setenv(tellomiai.EnvTranslate, "")
	t.Setenv(tellomiai.EnvCompose, "")
	t.Setenv(tellomiai.EnvAPIKey, "")
	if tellomiai.Advertised() {
		t.Fatal("advertised")
	}
	off := AppConfigHash()
	raw := mtproto.FromJSONValue(AppConfig())
	obj := raw.(map[string]any)
	if obj["tellomi_ai_enabled"] != false || obj["tellomi_ai_compose_enabled"] != false {
		t.Fatalf("flags %#v %#v", obj["tellomi_ai_enabled"], obj["tellomi_ai_compose_enabled"])
	}
	t.Setenv(tellomiai.EnvEnabled, "1")
	t.Setenv(tellomiai.EnvAPIKey, "test-key")
	if !tellomiai.Advertised() {
		t.Fatal("flag and key did not advertise")
	}
	if AppConfigHash() == off {
		t.Fatal("app config hash ignored TELLOMI_AI")
	}
	raw = mtproto.FromJSONValue(AppConfig())
	obj = raw.(map[string]any)
	if obj["tellomi_ai_enabled"] != true {
		t.Fatalf("tellomi_ai_enabled = %#v", obj["tellomi_ai_enabled"])
	}
	if obj["tellomi_ai_compose_enabled"] != false || obj["tellomi_translate_enabled"] != false {
		t.Fatalf("compose/translate followed TELLOMI_AI: %#v %#v", obj["tellomi_ai_compose_enabled"], obj["tellomi_translate_enabled"])
	}
}

func TestTranslateAndComposeFlags(t *testing.T) {
	t.Setenv(tellomiai.EnvEnabled, "")
	t.Setenv(tellomiai.EnvTranslate, "")
	t.Setenv(tellomiai.EnvCompose, "")
	t.Setenv(tellomiai.EnvAPIKey, "")
	if tellomiai.TranslateAdvertised() || tellomiai.ComposeAdvertised() {
		t.Fatal("advertised without a flag")
	}
	off := AppConfigHash()
	t.Setenv(tellomiai.EnvTranslate, "1")
	t.Setenv(tellomiai.EnvAPIKey, "test-key")
	if tellomiai.TranslateAdvertised() {
		t.Fatal("a Bailian key must not advertise translation")
	}
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(local.Close)
	t.Setenv(tellomiai.EnvTranslateURL, local.URL)
	if !tellomiai.TranslateAdvertised() || tellomiai.ComposeAdvertised() {
		t.Fatal("healthy sidecar did not advertise translation")
	}
	if AppConfigHash() == off {
		t.Fatal("app config hash ignored the local translator")
	}
	raw := mtproto.FromJSONValue(AppConfig())
	obj := raw.(map[string]any)
	if obj["tellomi_translate_enabled"] != true || obj["tellomi_ai_compose_enabled"] != false {
		t.Fatalf("translate %#v compose %#v", obj["tellomi_translate_enabled"], obj["tellomi_ai_compose_enabled"])
	}
	if obj["translations_manual_enabled"] != "enabled" || obj["translations_auto_enabled"] != "disabled" {
		t.Fatalf("manual %#v auto %#v", obj["translations_manual_enabled"], obj["translations_auto_enabled"])
	}
	t.Setenv(tellomiai.EnvCompose, "1")
	if !tellomiai.ComposeAdvertised() {
		t.Fatal("compose flag did not follow TELLOMI_AI_COMPOSE")
	}
	raw = mtproto.FromJSONValue(AppConfig())
	obj = raw.(map[string]any)
	if obj["tellomi_ai_compose_enabled"] != true || obj["tellomi_ai_enabled"] != false {
		t.Fatalf("compose %#v ai %#v", obj["tellomi_ai_compose_enabled"], obj["tellomi_ai_enabled"])
	}
}

func TestAnnounceFlagDefaultsOff(t *testing.T) {
	t.Setenv(announce.EnvEnabled, "")
	t.Setenv(announce.EnvKey, "")
	if announce.Advertised() {
		t.Fatal("advertised")
	}
	off := AppConfigHash()
	raw := mtproto.FromJSONValue(AppConfig())
	obj := raw.(map[string]any)
	if obj["tellomi_announce_enabled"] != false {
		t.Fatalf("flag %#v", obj["tellomi_announce_enabled"])
	}
	t.Setenv(announce.EnvEnabled, "1")
	t.Setenv(announce.EnvKey, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if !announce.Advertised() {
		t.Fatal("flag and key did not advertise")
	}
	if AppConfigHash() == off {
		t.Fatal("app config hash ignored TELLOMI_ANNOUNCE")
	}
	raw = mtproto.FromJSONValue(AppConfig())
	obj = raw.(map[string]any)
	if obj["tellomi_announce_enabled"] != true {
		t.Fatalf("tellomi_announce_enabled = %#v", obj["tellomi_announce_enabled"])
	}
}

func TestAboutDisclosureDefaultsOff(t *testing.T) {
	t.Setenv(about.EnvEnabled, "")
	t.Setenv(about.EnvAlgorithmFiling, "网信算备示例")
	t.Setenv(about.EnvAIModel, "Qwen")
	t.Setenv(about.EnvAIRegistration, "示例登记")
	off := AppConfigHash()
	raw := mtproto.FromJSONValue(AppConfig())
	obj := raw.(map[string]any)
	if obj["tellomi_personalized_recommend_default"] != false {
		t.Fatalf("default = %#v", obj["tellomi_personalized_recommend_default"])
	}
	for _, key := range []string{"tellomi_algorithm_filing_number", "tellomi_ai_model_name", "tellomi_ai_registration_number"} {
		if _, ok := obj[key]; ok {
			t.Fatalf("%s published while disclosure is off: %#v", key, obj[key])
		}
	}

	t.Setenv(about.EnvEnabled, "1")
	if AppConfigHash() == off {
		t.Fatal("app config hash ignored TELLOMI_ABOUT_DISCLOSURE")
	}
	raw = mtproto.FromJSONValue(AppConfig())
	obj = raw.(map[string]any)
	if obj["tellomi_personalized_recommend_default"] != true || obj["tellomi_algorithm_filing_number"] != "网信算备示例" {
		t.Fatalf("recommend %#v filing %#v", obj["tellomi_personalized_recommend_default"], obj["tellomi_algorithm_filing_number"])
	}
	if obj["tellomi_ai_model_name"] != "Qwen" || obj["tellomi_ai_registration_number"] != "示例登记" {
		t.Fatalf("model %#v reg %#v", obj["tellomi_ai_model_name"], obj["tellomi_ai_registration_number"])
	}
}

func TestStoriesVenueSearchUsername(t *testing.T) {
	obj := configMap(t)
	if obj["stories_venue_search_username"] != "venue" {
		t.Fatalf("stories_venue_search_username %#v", obj["stories_venue_search_username"])
	}
}

func TestHelpCenterFlag(t *testing.T) {
	t.Setenv(brand.EnvHelpCenter, "")
	raw := mtproto.FromJSONValue(AppConfig())
	obj := raw.(map[string]any)
	if obj["faq_link"] != brand.FAQURL {
		t.Fatalf("default faq_link = %#v", obj["faq_link"])
	}
	off := AppConfigHash()
	t.Setenv(brand.EnvHelpCenter, "1")
	if AppConfigHash() == off {
		t.Fatal("app config hash ignored TELLOMI_HELP_CENTER")
	}
	raw = mtproto.FromJSONValue(AppConfig())
	obj = raw.(map[string]any)
	if obj["faq_link"] != "https://tellomi.cn/help" {
		t.Fatalf("faq_link = %#v", obj["faq_link"])
	}
}

func TestCrashReportBotConfig(t *testing.T) {
	t.Setenv(crashreport.EnvBot, "")
	raw := mtproto.FromJSONValue(AppConfig())
	obj := raw.(map[string]any)
	if obj["tellomi_crash_report_bot"] != crashreport.DefaultUsername {
		t.Fatalf("bot %#v", obj["tellomi_crash_report_bot"])
	}
	off := AppConfigHash()
	t.Setenv(crashreport.EnvBot, "OtherBot")
	if AppConfigHash() == off {
		t.Fatal("app config hash ignored TELLOMI_CRASH_REPORT_BOT")
	}
	raw = mtproto.FromJSONValue(AppConfig())
	obj = raw.(map[string]any)
	if obj["tellomi_crash_report_bot"] != "OtherBot" {
		t.Fatalf("bot %#v", obj["tellomi_crash_report_bot"])
	}
}
