package e2e

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAnnounceFlagDefaultsOff(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	obj := invokeOK(t, a, &mtproto.TLHelpGetAppConfig61E3F854{Constructor: mtproto.CRC32_help_getAppConfig61E3F854})
	app, ok := obj.(*mtproto.TLHelpAppConfig)
	if !ok || app.GetData2() == nil {
		t.Fatalf("help.getAppConfig returned %T", obj)
	}
	on, found := jsonBoolField(app.GetData2().GetConfig(), "tellomi_announce_enabled")
	if !found {
		t.Fatal("help.getAppConfig is missing tellomi_announce_enabled")
	}
	if on {
		t.Fatal("tellomi_announce_enabled is true; the launch notice must stay off")
	}
}

func jsonBoolField(v *mtproto.JSONValue, key string) (bool, bool) {
	if v == nil {
		return false, false
	}
	for _, pair := range v.GetValue_VECTORJSONOBJECTVALUE() {
		if pair.GetKey() != key || pair.GetValue() == nil {
			continue
		}
		b := pair.GetValue().GetValue_BOOL()
		if b == nil {
			return false, false
		}
		return mtproto.FromBool(b), true
	}
	return false, false
}
