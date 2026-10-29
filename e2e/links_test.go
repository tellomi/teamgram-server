package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestLinksUseTellCC(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	a.SetProfile(ProfileIOS)
	obj := invokeOK(t, a, &mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
	cfg, ok := obj.(*mtproto.TLConfig)
	if !ok {
		t.Fatalf("help.getConfig returned %T", obj)
	}
	prefix := cfg.GetMeUrlPrefix()
	if !strings.Contains(prefix, "tell.cc") {
		t.Fatalf("me_url_prefix = %q, want a tell.cc link", prefix)
	}
	for _, banned := range []string{"t.me", "telegram.me", "telegram.dog"} {
		if strings.Contains(prefix, banned) {
			t.Fatalf("me_url_prefix %q still points at %s", prefix, banned)
		}
	}

	app := invokeOK(t, a, &mtproto.TLHelpGetAppConfig61E3F854{Constructor: mtproto.CRC32_help_getAppConfig61E3F854})
	dump := fmt.Sprintf("%+v", app)
	if strings.Contains(dump, "https://t.me") || strings.Contains(dump, "https://telegram.me") {
		t.Fatalf("help.getAppConfig still advertises a Telegram short link: %s", dump)
	}
}
