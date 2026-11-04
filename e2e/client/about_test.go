package e2e

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAboutDisclosureDefaultsOff(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	obj := invokeOK(t, a, &mtproto.TLHelpGetAppConfig61E3F854{Constructor: mtproto.CRC32_help_getAppConfig61E3F854})
	app, ok := obj.(*mtproto.TLHelpAppConfig)
	if !ok || app.GetData2() == nil {
		t.Fatalf("help.getAppConfig returned %T", obj)
	}
	cfg := app.GetData2().GetConfig()
	on, found := jsonBoolField(cfg, "tellomi_personalized_recommend_default")
	if !found {
		t.Fatal("help.getAppConfig is missing tellomi_personalized_recommend_default")
	}
	if on {
		t.Fatal("tellomi_personalized_recommend_default is true; disclosure must stay off")
	}
	for _, key := range []string{"tellomi_algorithm_filing_number", "tellomi_ai_model_name", "tellomi_ai_registration_number"} {
		if _, ok := jsonStringField(cfg, key); ok {
			t.Fatalf("help.getAppConfig published %s while disclosure is off", key)
		}
	}
}

func jsonStringField(v *mtproto.JSONValue, key string) (string, bool) {
	if v == nil {
		return "", false
	}
	for _, pair := range v.GetValue_VECTORJSONOBJECTVALUE() {
		if pair.GetKey() != key || pair.GetValue() == nil {
			continue
		}
		s := pair.GetValue().GetValue_STRING()
		if s == "" && pair.GetValue().GetPredicateName() != mtproto.Predicate_jsonString {
			return "", false
		}
		return s, true
	}
	return "", false
}
