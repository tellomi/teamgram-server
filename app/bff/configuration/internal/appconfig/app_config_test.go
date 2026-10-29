package appconfig

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

func TestAppConfigPremiumPolicy(t *testing.T) {
	t.Setenv(premium.EnvEveryone, "true")
	t.Setenv(premium.EnvFreeFeatures, "")
	t.Setenv(premium.EnvPurchaseBlocked, "true")
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
}
