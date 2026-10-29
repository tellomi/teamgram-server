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
	if len(langs.Datas) != 3 || langs.Datas[0].GetLangCode() != "zh-hans" {
		t.Fatalf("%+v", langs.Datas)
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
	if _, err := c.LangpackGetLanguage(&mtproto.TLLangpackGetLanguage{LangPack: "android", LangCode: "de"}); err != mtproto.ErrLangCodeNotSupported {
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
