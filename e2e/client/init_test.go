package e2e

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestInitConnectionInvokeWithLayer(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	for _, profile := range []Profile{ProfileIOS, ProfileAndroid} {
		t.Run(profile.LangPack, func(t *testing.T) {
			a.SetProfile(profile)
			obj := invokeOK(t, a, &mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
			cfg, ok := obj.(*mtproto.TLConfig)
			if !ok {
				t.Fatalf("help.getConfig returned %T", obj)
			}
			if cfg.GetThisDc() == 0 && cfg.To_Config().GetThisDc() == 0 {
				t.Fatal("config.this_dc is 0")
			}
		})
	}
	a.SetProfile(ProfileIOS)
}

func TestHelpGetConfig(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	a.SetProfile(ProfileIOS)
	obj := invokeOK(t, a, &mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
	cfg, ok := obj.(*mtproto.TLConfig)
	if !ok {
		t.Fatalf("returned %T", obj)
	}
	if cfg.GetMeUrlPrefix() == "" {
		t.Fatal("me_url_prefix is empty")
	}
}

func TestHelpGetAppConfig(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	obj := invokeOK(t, a, &mtproto.TLHelpGetAppConfig61E3F854{Constructor: mtproto.CRC32_help_getAppConfig61E3F854})
	switch obj.(type) {
	case *mtproto.TLHelpAppConfig, *mtproto.TLHelpAppConfigNotModified:
	default:
		t.Fatalf("returned %T", obj)
	}
}

func TestUpdatesGetStateAndDifference(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	c, cancel := ctx()
	defer cancel()
	pts, qts, date, err := a.State(c)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := a.Difference(c, pts, qts, date)
	if err != nil {
		t.Fatal(err)
	}
	switch diff.(type) {
	case *mtproto.TLUpdatesDifference, *mtproto.TLUpdatesDifferenceSlice, *mtproto.TLUpdatesDifferenceEmpty, *mtproto.TLUpdatesDifferenceTooLong:
	default:
		t.Fatalf("updates.getDifference returned %T", diff)
	}
}
