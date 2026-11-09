package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/langpack/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/langpack"
	"github.com/zeromicro/go-zero/core/logx"
)

func testCore(md *metadata.RpcMetadata) *LangpackCore {
	return &LangpackCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Catalog: langpack.DefaultCatalog()},
		Logger: logx.WithContext(context.Background()),
		MD:     md,
	}
}

func TestLoginLanguageSwitchWorksBeforeSignIn(t *testing.T) {
	c := testCore(nil)
	langs, err := c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{LangPack: "android"})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, lang := range langs.Datas {
		seen[lang.GetLangCode()] = true
	}
	if !seen["zh-hans"] || !seen["en"] {
		t.Fatalf("login list missing zh-hans or en: %v", seen)
	}
	zh, err := c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangPack: "android", LangCode: "zh-hans"})
	if err != nil || zh.GetLangCode() != "zh-hans" || len(zh.GetStrings()) < 1000 {
		t.Fatalf("zh pack %s %d %v", zh.GetLangCode(), len(zh.GetStrings()), err)
	}
	en, err := c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangPack: "ios", LangCode: "en"})
	if err != nil || en.GetLangCode() != "en" || len(en.GetStrings()) < 1000 {
		t.Fatalf("en pack %s %d %v", en.GetLangCode(), len(en.GetStrings()), err)
	}
	zhStr, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{LangPack: "android", LangCode: "zh-CN", Keys: []string{"ChooseYourLanguage"}})
	if err != nil || len(zhStr.Datas) != 1 || zhStr.Datas[0].GetValue() == "" {
		t.Fatalf("zh string %+v %v", zhStr, err)
	}
	enStr, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{LangPack: "android", LangCode: "en", Keys: []string{"ChooseYourLanguage"}})
	if err != nil || len(enStr.Datas) != 1 || enStr.Datas[0].GetValue() == "" {
		t.Fatalf("en string %+v %v", enStr, err)
	}
	if zhStr.Datas[0].GetValue() == enStr.Datas[0].GetValue() {
		t.Fatalf("language switch would not change ChooseYourLanguage: %q", zhStr.Datas[0].GetValue())
	}
	diff, err := c.LangpackGetDifference(&mtproto.TLLangpackGetDifference{LangPack: "android", LangCode: "en", FromVersion: 0})
	if err != nil || diff.GetLangCode() != "en" || len(diff.GetStrings()) < 1000 {
		t.Fatalf("difference %s %d %v", diff.GetLangCode(), len(diff.GetStrings()), err)
	}
}

func TestGetLanguagesUsesConnectionLangPack(t *testing.T) {
	t.Setenv("TELLOMI_LANGPACK_HIDDEN", "hi,th,el")
	c := testCore(&metadata.RpcMetadata{Langpack: "ios", LangCode: "en", SystemLangCode: "zh-Hans"})
	langs, err := c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{})
	if err != nil {
		t.Fatal(err)
	}
	if len(langs.Datas) != offeredOfficialCount() || langs.Datas[0].GetLangCode() != "zh-hans" {
		t.Fatalf("%d %+v", len(langs.Datas), langs.Datas[0])
	}
	for _, lang := range langs.Datas {
		switch lang.GetLangCode() {
		case "hi", "th", "el":
			t.Fatalf("offered %s", lang.GetLangCode())
		}
	}
	if _, err := c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{LangPack: "nope"}); err != mtproto.ErrLangPackInvalid {
		t.Fatal(err)
	}
}

func TestGetLangPackAliasesAndDifference(t *testing.T) {
	c := testCore(&metadata.RpcMetadata{Langpack: "android"})
	diff, err := c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangCode: "zh"})
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetLangCode() != "zh-hans" || diff.GetFromVersion() != 0 || len(diff.GetStrings()) == 0 {
		t.Fatalf("%+v", diff)
	}
	for _, s := range diff.GetStrings() {
		if s.GetKey() == "AppName" && s.GetValue() != "Tellomi" {
			t.Fatalf("AppName %q", s.GetValue())
		}
	}
	next, err := c.LangpackGetDifference(&mtproto.TLLangpackGetDifference{LangCode: "zh-hans", FromVersion: diff.GetVersion()})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.GetStrings()) != 0 || next.GetVersion() != diff.GetVersion() {
		t.Fatalf("difference %+v", next)
	}
	de, err := c.LangpackGetLanguage(&mtproto.TLLangpackGetLanguage{LangPack: "android", LangCode: "de"})
	if err != nil {
		t.Fatal(err)
	}
	if de.GetLangCode() != "de" || de.GetBaseLangCode().GetValue() != "en" || de.GetNativeName() != "Deutsch" {
		t.Fatalf("%+v", de)
	}
	// A phone locale is regional. getLanguage must not error, and the
	// response code stays the catalog code the language list already showed.
	for _, tag := range []string{"de-DE", "fr-FR", "ja-JP", "en-US", "zh_Hans"} {
		one, err := c.LangpackGetLanguage(&mtproto.TLLangpackGetLanguage{LangPack: "ios", LangCode: tag})
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		pack, err := c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangPack: "android", LangCode: tag})
		if err != nil || pack.GetFromVersion() != 0 || len(pack.GetStrings()) < 1000 {
			t.Fatalf("%s pack err=%v code=%s n=%d", tag, err, pack.GetLangCode(), len(pack.GetStrings()))
		}
		if pack.GetLangCode() != one.GetLangCode() {
			t.Fatalf("%s language %s pack %s", tag, one.GetLangCode(), pack.GetLangCode())
		}
		diff, err := c.LangpackGetDifference(&mtproto.TLLangpackGetDifference{LangPack: "ios", LangCode: tag, FromVersion: 0})
		if err != nil || diff.GetLangCode() != pack.GetLangCode() || len(diff.GetStrings()) < 1000 {
			t.Fatalf("%s difference err=%v code=%s n=%d", tag, err, diff.GetLangCode(), len(diff.GetStrings()))
		}
	}
}

func TestLangPackWithoutSession(t *testing.T) {
	c := testCore(nil)
	langs, err := c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{LangPack: "android"})
	if err != nil {
		t.Fatal(err)
	}
	if len(langs.Datas) < 20 || langs.Datas[0].GetLangCode() != "zh-hans" {
		t.Fatalf("languages %d %+v", len(langs.Datas), langs.Datas[0])
	}
	one, err := c.LangpackGetLanguage(&mtproto.TLLangpackGetLanguage{LangPack: "ios", LangCode: "ja"})
	if err != nil || one.GetLangCode() != "ja" || one.GetNativeName() != "日本語" || one.GetTranslatedCount() < 1000 {
		t.Fatalf("%+v %v", one, err)
	}
	diff, err := c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangPack: "android", LangCode: "de"})
	if err != nil || diff.GetLangCode() != "de" || len(diff.GetStrings()) < 1000 {
		t.Fatalf("de pack %d %v", len(diff.GetStrings()), err)
	}
	got, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{LangPack: "android", LangCode: "ja", Keys: []string{"LanguageName"}})
	if err != nil || got.Datas[0].GetValue() != "日本語" {
		t.Fatalf("%+v %v", got, err)
	}
	ja, err := c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangPack: "android", LangCode: "ja"})
	if err != nil || ja.GetLangCode() != "ja" || len(ja.GetStrings()) < 1000 {
		t.Fatalf("ja pack %d %v", len(ja.GetStrings()), err)
	}
	same, err := c.LangpackGetDifference(&mtproto.TLLangpackGetDifference{LangPack: "android", LangCode: "ja", FromVersion: ja.GetVersion()})
	if err != nil || len(same.GetStrings()) != 0 {
		t.Fatalf("caught up %d %v", len(same.GetStrings()), err)
	}
	if _, err := c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{LangPack: "nope"}); err != mtproto.ErrLangPackInvalid {
		t.Fatal(err)
	}
}

func TestRemovedChinesePackFallsBack(t *testing.T) {
	c := testCore(&metadata.RpcMetadata{Langpack: "android"})
	langs, err := c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{LangPack: "ios"})
	if err != nil {
		t.Fatal(err)
	}
	var simplified, traditional int
	for _, lang := range langs.Datas {
		switch lang.GetLangCode() {
		case "classic-zh-cn", "taiwan":
			t.Fatalf("listed %s", lang.GetLangCode())
		case "zh-hans":
			simplified++
		case "zh-hant":
			traditional++
		}
		if lang.GetNativeName() == "简体中文" && lang.GetLangCode() != "zh-hans" {
			t.Fatalf("extra simplified %+v", lang)
		}
		if lang.GetNativeName() == "正體中文" {
			t.Fatal("正體中文 still listed")
		}
	}
	if simplified != 1 || traditional != 1 {
		t.Fatalf("simplified %d traditional %d of %d", simplified, traditional, len(langs.Datas))
	}

	pack, err := c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangPack: "ios", LangCode: "classic-zh-cn"})
	if err != nil {
		t.Fatal(err)
	}
	official, err := c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangPack: "ios", LangCode: "zh-hans"})
	if err != nil {
		t.Fatal(err)
	}
	if pack.GetLangCode() != "zh-hans" || pack.GetVersion() != official.GetVersion() || len(pack.GetStrings()) != len(official.GetStrings()) || len(pack.GetStrings()) < 1000 {
		t.Fatalf("classic pack code=%s v=%d n=%d official n=%d", pack.GetLangCode(), pack.GetVersion(), len(pack.GetStrings()), len(official.GetStrings()))
	}
	got, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{LangPack: "android", LangCode: "classic-zh-cn", Keys: []string{"Settings"}})
	if err != nil {
		t.Fatal(err)
	}
	want, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{LangPack: "android", LangCode: "zh-hans", Keys: []string{"Settings"}})
	if err != nil || got.Datas[0].GetValue() == "" || got.Datas[0].GetValue() != want.Datas[0].GetValue() {
		t.Fatalf("classic strings %+v %+v %v", got, want, err)
	}
	tw, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{LangPack: "android", LangCode: "taiwan", Keys: []string{"LanguageName"}})
	if err != nil {
		t.Fatal(err)
	}
	hant, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{LangPack: "android", LangCode: "zh-hant", Keys: []string{"LanguageName"}})
	if err != nil || tw.Datas[0].GetValue() == "" || tw.Datas[0].GetValue() != hant.Datas[0].GetValue() {
		t.Fatalf("taiwan strings %+v %+v %v", tw, hant, err)
	}
	one, err := c.LangpackGetLanguage(&mtproto.TLLangpackGetLanguage{LangPack: "ios", LangCode: "taiwan"})
	if err != nil || one.GetLangCode() != "zh-hant" || one.GetNativeName() != "繁體中文" || one.GetBaseLangCode().GetValue() != "en" {
		t.Fatalf("taiwan language %+v %v", one, err)
	}
}

func offeredOfficialCount() int {
	n := 0
	for _, lang := range langpack.OfficialLanguages {
		if !langpack.HiddenLang(lang.Code) {
			n++
		}
	}
	return n
}

func TestHiddenLanguagesStayAvailable(t *testing.T) {
	t.Setenv("TELLOMI_LANGPACK_HIDDEN", "hi,th,el")
	c := testCore(nil)
	for _, pack := range []string{"android", "ios", "tdesktop", "macos"} {
		langs, err := c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{LangPack: pack})
		if err != nil {
			t.Fatal(pack, err)
		}
		if len(langs.Datas) != len(langpack.OfficialLanguages)-3 {
			t.Fatalf("%s languages %d", pack, len(langs.Datas))
		}
		for _, lang := range langs.Datas {
			switch lang.GetLangCode() {
			case "hi", "th", "el":
				t.Fatalf("%s offered %s", pack, lang.GetLangCode())
			}
		}
	}
	for _, code := range []string{"hi", "th", "el"} {
		diff, err := c.LangpackGetLangPack(&mtproto.TLLangpackGetLangPack{LangPack: "android", LangCode: code})
		if err != nil || diff.GetLangCode() != code {
			t.Fatalf("getLangPack %s %+v %v", code, diff, err)
		}
		one, err := c.LangpackGetLanguage(&mtproto.TLLangpackGetLanguage{LangPack: "ios", LangCode: code})
		if err != nil || one.GetLangCode() != code {
			t.Fatalf("getLanguage %s %+v %v", code, one, err)
		}
		got, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{LangPack: "android", LangCode: code, Keys: []string{"LanguageName"}})
		if err != nil || len(got.Datas) != 1 {
			t.Fatalf("getStrings %s %+v %v", code, got, err)
		}
	}
	hi, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{LangPack: "android", LangCode: "hi", Keys: []string{"Settings"}})
	if err != nil || hi.Datas[0].GetValue() != "सेटिंग्स" {
		t.Fatalf("hindi settings %+v %v", hi, err)
	}
}

func TestGetStringsForLanguageAlert(t *testing.T) {
	c := testCore(&metadata.RpcMetadata{Langpack: "android", LangCode: "zh-CN"})
	out, err := c.LangpackGetStrings(&mtproto.TLLangpackGetStrings{Keys: []string{"Settings", "NoSuchKey"}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Datas[0].GetValue() != "设置" {
		t.Fatal(out.Datas[0].GetValue())
	}
	if out.Datas[1].GetPredicateName() != mtproto.Predicate_langPackStringDeleted {
		t.Fatal(out.Datas[1].GetPredicateName())
	}
}
