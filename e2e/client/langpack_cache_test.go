package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestLangPackWarmIsFast is langpack.getLangPack twice. The cold build may
// be slow; the second call must be the cached pack. zh-hans and en stay
// different so the login language switch still works.
func TestLangPackWarmIsFast(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, err := Dial(ctx, cfg, ProfileAndroid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)

	req := &mtproto.TLLangpackGetLangPack{
		Constructor: mtproto.CRC32_langpack_getLangPack_f2f2330a,
		LangPack:    "android",
		LangCode:    "zh-hans",
	}
	start := time.Now()
	firstObj := invokeOK(t, c, req)
	cold := time.Since(start)
	start = time.Now()
	secondObj := invokeOK(t, c, req)
	warm := time.Since(start)
	first, ok := firstObj.(*mtproto.TLLangPackDifference)
	second, ok2 := secondObj.(*mtproto.TLLangPackDifference)
	if !ok || !ok2 || first.GetVersion() == 0 || len(first.GetStrings()) < 1000 {
		t.Fatalf("pack %T %T", firstObj, secondObj)
	}
	if second.GetVersion() != first.GetVersion() || len(second.GetStrings()) != len(first.GetStrings()) {
		t.Fatalf("warm version %d/%d strings %d/%d", second.GetVersion(), first.GetVersion(), len(second.GetStrings()), len(first.GetStrings()))
	}
	if warm > time.Second && warm > cold/2 {
		t.Fatalf("cached getLangPack took %s (cold %s)", warm, cold)
	}
	t.Logf("getLangPack cold %s warm %s strings %d", cold, warm, len(first.GetStrings()))

	diff := invokeOK(t, c, &mtproto.TLLangpackGetDifference{
		Constructor: mtproto.CRC32_langpack_getDifference,
		LangPack:    "android",
		LangCode:    "zh-hans",
		FromVersion: first.GetVersion(),
	})
	empty, ok := diff.(*mtproto.TLLangPackDifference)
	if !ok || empty.GetVersion() != first.GetVersion() || len(empty.GetStrings()) != 0 {
		t.Fatalf("current client difference %+v", diff)
	}

	zh := oneString(t, c, "android", "zh-hans", "ChooseYourLanguage")
	en := oneString(t, c, "android", "en", "ChooseYourLanguage")
	if zh == "" || en == "" || zh == en {
		t.Fatalf("language switch zh %q en %q", zh, en)
	}
}

func oneString(t *testing.T, c *Client, pack, code, key string) string {
	t.Helper()
	res := invokeOK(t, c, &mtproto.TLLangpackGetStrings{
		Constructor: mtproto.CRC32_langpack_getStrings_efea3803,
		LangPack:    pack,
		LangCode:    code,
		Keys:        []string{key},
	})
	vec, ok := res.(*tlVector)
	if !ok || len(vec.Strings) != 1 {
		t.Fatalf("getStrings %T", res)
	}
	return vec.Strings[0].GetValue()
}
