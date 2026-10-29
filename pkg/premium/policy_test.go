package premium

import (
	"testing"
	"time"
)

func TestDefaultTreatsEveryoneAsMember(t *testing.T) {
	t.Setenv(EnvEveryone, "")
	t.Setenv(EnvFreeFeatures, "")
	t.Setenv(EnvPurchaseBlocked, "")
	cfg := Reload()
	t.Cleanup(func() { Reload() })

	if !cfg.Everyone || !cfg.PurchaseBlocked {
		t.Fatalf("config %#v", cfg)
	}
	if len(cfg.Free) != 0 {
		t.Fatalf("default features should be paid, got %#v", cfg.Free)
	}
	now := time.Now().Unix()
	if !IsMember(false, 0, now) {
		t.Fatal("global switch did not grant membership")
	}
	if !ActiveSubscription(true, 0, now) {
		t.Fatal("flag without expiry is not an active subscription")
	}
	if ActiveSubscription(true, now-10, now) {
		t.Fatal("expired subscription was active")
	}
	if !Allows(FeatureAbout, false) {
		t.Fatal("member-by-switch should allow a paid feature")
	}
	def, prem := Advertised(FeatureAbout)
	if def != AboutLength || prem != AboutLength {
		t.Fatalf("advertised about %d %d", def, prem)
	}
}

func TestPaidFeaturesFollowTheDatabaseRow(t *testing.T) {
	t.Setenv(EnvEveryone, "false")
	t.Setenv(EnvFreeFeatures, "emoji_status")
	cfg := Reload()
	t.Cleanup(func() { Reload() })
	if cfg.Everyone || !cfg.Free[FeatureEmojiStatus] || cfg.Free[FeatureAbout] {
		t.Fatalf("config %#v", cfg)
	}
	now := time.Now().Unix()
	if IsMember(false, 0, now) {
		t.Fatal("non-subscriber was a member with the global switch off")
	}
	if !IsMember(true, now+60, now) {
		t.Fatal("unexpired subscriber was not a member")
	}
	if !Allows(FeatureEmojiStatus, false) {
		t.Fatal("free feature was gated")
	}
	if Allows(FeatureAbout, false) {
		t.Fatal("paid feature was allowed for a non-member")
	}
	if !Allows(FeatureAbout, true) {
		t.Fatal("paid feature was refused for a member")
	}
	if Limit(FeatureAbout, false) != AboutLengthFree || Limit(FeatureAbout, true) != AboutLength {
		t.Fatal("about limits did not follow membership")
	}
	def, prem := Advertised(FeaturePinnedChats)
	if def != PinnedDialogsFree || prem != PinnedDialogs {
		t.Fatalf("advertised pins %d %d", def, prem)
	}
	if AboutWithinLimit(string(make([]rune, AboutLength)), false) {
		t.Fatal("non-member biography used the Premium length")
	}
	if !AboutWithinLimit(string(make([]rune, AboutLength)), true) {
		t.Fatal("member biography was rejected at the Premium length")
	}
}

func TestStarMarksEveryFeatureFree(t *testing.T) {
	t.Setenv(EnvEveryone, "false")
	t.Setenv(EnvFreeFeatures, "*")
	Reload()
	t.Cleanup(func() { Reload() })
	if !Allows(FeatureVoiceToText, false) || !Allows(FeatureProfileColor, false) {
		t.Fatal("* did not mark features free")
	}
	if IsMember(false, 0, time.Now().Unix()) {
		t.Fatal("free features granted the Premium badge")
	}
}
