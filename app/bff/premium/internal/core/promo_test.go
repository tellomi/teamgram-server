package core

import "testing"

func TestPremiumPromoHasNoPurchase(t *testing.T) {
	promo := PremiumPromo()
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
