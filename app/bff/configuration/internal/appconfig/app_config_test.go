package appconfig

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/pkg/announce"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
	"github.com/teamgram/teamgram-server/pkg/premium"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
)

func TestAppConfigPremiumPolicy(t *testing.T) {
	t.Cleanup(func() { emojiblock.Reload() })
	t.Setenv(emojiblock.EnvExtra, "")
	t.Setenv(emojiblock.EnvFile, "")
	emojiblock.Reload()
	t.Setenv(premium.EnvEveryone, "true")
	t.Setenv(premium.EnvFreeFeatures, "")
	t.Setenv(premium.EnvPurchaseBlocked, "true")
	t.Setenv(EnvTranscribeEnabled, "")
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
		"premium_purchase_blocked":           true,
		"stars_purchase_blocked":             true,
		"stars_gifts_enabled":                false,
		"stargifts_blocked":                  true,
		"enable_gifts_in_profile":            false,
		"premium_gift_attach_menu_icon":      false,
		"premium_gift_text_field_icon":       false,
		"stars_paid_messages_available":      false,
		"giveaway_gifts_purchase_available":  false,
		"channel_revenue_withdrawal_enabled": false,
		"starref_program_allowed":            false,
		"starref_connect_allowed":            false,
		"wallet_available":                   false,
		"tellomi_ai_compose_enabled":         false,
		"tellomi_ai_enabled":                 false,
		"tellomi_translate_enabled":          false,
		"tellomi_announce_enabled":           false,
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
