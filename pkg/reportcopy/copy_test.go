package reportcopy

import (
	"testing"
	"unicode"

	"github.com/teamgram/proto/mtproto"
)

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func TestChooseLanguages(t *testing.T) {
	en := Choose("en", Story)
	if en.GetPredicateName() != mtproto.Predicate_reportResultChooseOption || en.GetTitle() != "Report story" {
		t.Fatalf("en story %#v", en)
	}
	if len(en.GetOptions()) != 4 || en.GetOptions()[0].GetText() != "Spam" || string(en.GetOptions()[3].GetOption()) != "other" {
		t.Fatalf("en options %#v", en.GetOptions())
	}

	hans := Choose("zh-hans", Story)
	if hans.GetTitle() != "举报动态" || !hasHan(hans.GetTitle()) {
		t.Fatalf("hans title %q", hans.GetTitle())
	}
	for _, opt := range hans.GetOptions() {
		if !hasHan(opt.GetText()) || len(opt.GetOption()) == 0 {
			t.Fatalf("hans option %#v", opt)
		}
	}
	if hans.GetOptions()[0].GetText() != "垃圾信息" || string(hans.GetOptions()[2].GetOption()) != "illegal" {
		t.Fatalf("hans ids %#v", hans.GetOptions())
	}

	hant := Choose("zh-Hant", Message)
	if hant.GetTitle() != "這則訊息有什麼問題？" || hant.GetOptions()[0].GetText() != "垃圾訊息" {
		t.Fatalf("hant %#v", hant)
	}
	if len(hant.GetOptions()) != 7 || string(hant.GetOptions()[3].GetOption()) != "child" {
		t.Fatalf("message ids %#v", hant.GetOptions())
	}

	for _, code := range []string{"", "ja", "fr", "en-US", "de"} {
		got := Choose(code, Story)
		if got.GetTitle() != "Report story" || got.GetOptions()[0].GetText() != "Spam" {
			t.Fatalf("%s fell back to %#v", code, got)
		}
	}
	if Choose("zh_CN", Message).GetTitle() != "这条消息有什么问题？" {
		t.Fatal("zh_CN")
	}
	if Choose("zh-TW", Story).GetTitle() != "舉報動態" {
		t.Fatal("zh-TW")
	}
}
