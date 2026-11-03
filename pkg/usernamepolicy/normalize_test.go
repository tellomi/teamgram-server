package usernamepolicy

import "testing"

func TestNormalizeCaseAndWidth(t *testing.T) {
	if got := Normalize("ＡＤＭＩＮ"); got != "admin" {
		t.Fatalf("fullwidth %q", got)
	}
	if got := Normalize("Admin"); got != "admin" {
		t.Fatalf("case %q", got)
	}
	if got := Normalize("straße"); got != "strasse" {
		t.Fatalf("eszett %q", got)
	}
	if got := Normalize("  Tellomi\u00A0\u3000Support  "); got != "tellomi support" {
		t.Fatalf("space %q", got)
	}
}

func TestControlRejected(t *testing.T) {
	for _, s := range []string{"ad\u200Bmin", "\uFEFFadmin", "ad\u202Emin"} {
		if !HasDisallowedControl(s) {
			t.Fatalf("accepted %q", s)
		}
	}
	if HasDisallowedControl("admin") || HasDisallowedControl("小明") {
		t.Fatal("plain text flagged")
	}
}

func TestSkeletonLookalikes(t *testing.T) {
	base := Skeleton(Normalize("tellomi"))
	for _, s := range []string{"tel1omi", "te11omi", "tellorni", "t3llomi", "tellomi_", "t\u0435llomi"} {
		if got := Skeleton(Normalize(s)); got != base {
			t.Fatalf("%q skeleton %q, tellomi %q", s, got, base)
		}
	}
	if Skeleton(Normalize("telegram")) == base || Skeleton(Normalize("hello")) == base {
		t.Fatal("unrelated word collided with tellomi")
	}
}

func TestMixedScript(t *testing.T) {
	if !MixedScript(Normalize("t\u0435llomi")) {
		t.Fatal("cyrillic lookalike not mixed")
	}
	for _, s := range []string{"tellomi", "小明", "bob 李", "bob李", "alice chen", "咖啡爱好者"} {
		if MixedScript(Normalize(s)) {
			t.Fatalf("ordinary name flagged: %q", s)
		}
	}
}
