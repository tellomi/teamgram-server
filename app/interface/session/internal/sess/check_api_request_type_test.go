package sess

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestLangpackRPCsDoNotRequireLogin(t *testing.T) {
	calls := []mtproto.TLObject{
		&mtproto.TLLangpackGetLanguages{LangPack: "ios"},
		&mtproto.TLLangpackGetLanguage{LangPack: "android", LangCode: "ja"},
		&mtproto.TLLangpackGetLangPack{LangPack: "android", LangCode: "zh-hans"},
		&mtproto.TLLangpackGetStrings{LangPack: "ios", LangCode: "en", Keys: []string{"Settings.Title"}},
		&mtproto.TLLangpackGetDifference{LangPack: "tdesktop", LangCode: "ru", FromVersion: 0},
	}
	for _, call := range calls {
		if !checkRpcWithoutLogin(call) {
			t.Fatalf("%T is not callable before login", call)
		}
	}
	if checkRpcWithoutLogin(&mtproto.TLMessagesGetDialogs{}) {
		t.Fatal("messages.getDialogs must stay behind login")
	}
}
