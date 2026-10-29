package appconfig

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
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
	if AppConfigHash() != premium.AppConfigHash() || AppConfigHash() == 0 {
		t.Fatal("app config hash did not follow the loaded policy")
	}
}
