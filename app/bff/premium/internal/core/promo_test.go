package core

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/pkg/langpack"
)

func TestPremiumPromoHasNoPurchase(t *testing.T) {
	promo := PremiumPromo("")
	if promo.GetStatusText() == "" {
		t.Fatal("premium promo status text is empty")
	}
	if len(promo.GetPeriodOptions()) != 0 {
		t.Fatalf("period options = %d", len(promo.GetPeriodOptions()))
	}
	if len(promo.GetVideoSections()) != len(promo.GetVideos()) {
		t.Fatal("video sections and videos differ in length")
	}
	if promo.GetMonthlyAmount() != 0 {
		t.Fatalf("monthly amount = %d", promo.GetMonthlyAmount())
	}
}

func TestPremiumPromoUsesClientLanguage(t *testing.T) {
	en := PremiumPromo("en").GetStatusText()
	if en != "Tellomi includes Telegram Premium for every account. There is nothing to buy." {
		t.Fatal(en)
	}
	ja := PremiumPromo("ja-JP").GetStatusText()
	if ja == en || !strings.Contains(ja, "購入") {
		t.Fatal(ja)
	}
	hant := PremiumPromo("zh-Hant-HK").GetStatusText()
	hans := PremiumPromo("zh-CN").GetStatusText()
	if hans == en || hant == en || hans == hant || !strings.Contains(hant, "無需購買") {
		t.Fatalf("hans %q hant %q", hans, hant)
	}
	for _, lang := range langpack.OfficialLanguages {
		text := PremiumPromo(lang.Code).GetStatusText()
		if text == "" {
			t.Fatalf("%s empty", lang.Code)
		}
		if lang.Code != "en" && text == en {
			t.Fatalf("%s fell back to English", lang.Code)
		}
	}
	c := &PremiumCore{MD: &metadata.RpcMetadata{Langpack: "zh-hant", SystemLangCode: "en"}}
	got, err := c.HelpGetPremiumPromo(nil)
	if err != nil || got.GetStatusText() != hant {
		t.Fatalf("handler %q %v", got.GetStatusText(), err)
	}
}
