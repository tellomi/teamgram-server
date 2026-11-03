package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestHelpGetConfigSuggestedLangFromSystem(t *testing.T) {
	restoreConfig(t)
	loadCheckedInConfig(t)
	t.Setenv("TELLOMI_LANGPACK_HIDDEN", "hi,th,el")
	t.Setenv("TELLOMI_DC_OPTIONS", "")
	t.Setenv("TELLOMI_DC_TXT_DOMAIN", "")

	cases := []struct {
		lang, system, want string
	}{
		{"en", "fr", "fr"},
		{"en", "fr-FR", "fr"},
		{"zh-hans", "en", "en"},
		{"de", "", "de"},
		{"en", "hi", "en"},
		{"ja", "th", "ja"},
		{"en", "el-GR", "en"},
		{"", "zh-Hant", "zh-hant"},
	}
	for _, tc := range cases {
		c := &ConfigurationCore{
			ctx: context.Background(),
			MD: &metadata.RpcMetadata{
				LangCode:       tc.lang,
				SystemLangCode: tc.system,
				Langpack:       "android",
			},
		}
		got, err := c.HelpGetConfig(&mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
		if err != nil {
			t.Fatal(err)
		}
		if got.GetSuggestedLangCode().GetValue() != tc.want {
			t.Fatalf("lang=%q system=%q suggested=%q want=%q", tc.lang, tc.system, got.GetSuggestedLangCode().GetValue(), tc.want)
		}
	}
}
