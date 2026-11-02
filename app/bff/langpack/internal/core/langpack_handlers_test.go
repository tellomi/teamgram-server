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

func TestGetLanguagesUsesConnectionLangPack(t *testing.T) {
	c := testCore(&metadata.RpcMetadata{Langpack: "ios", LangCode: "en", SystemLangCode: "zh-Hans"})
	langs, err := c.LangpackGetLanguages(&mtproto.TLLangpackGetLanguages{})
	if err != nil {
		t.Fatal(err)
	}
	if len(langs.Datas) != len(langpack.OfficialLanguages) || langs.Datas[0].GetLangCode() != "zh-hans" {
		t.Fatalf("%d %+v", len(langs.Datas), langs.Datas[0])
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
