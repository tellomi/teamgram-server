package core

import (
	"strings"
	"testing"
	"time"
)

func TestLoginSecurityNoticeHasDeviceAndBeijingTime(t *testing.T) {
	when := time.Date(2026, 11, 7, 2, 30, 0, 0, time.UTC)
	text, entities := loginSecurityNotice("iPhone 16", when)
	if !strings.Contains(text, "新设备登录提醒") {
		t.Fatalf("title missing: %q", text)
	}
	if !strings.Contains(text, "2026-11-07 10:30") {
		t.Fatalf("beijing time missing: %q", text)
	}
	if !strings.Contains(text, "iPhone 16") {
		t.Fatalf("device missing: %q", text)
	}
	for _, banned := range []string{"验证码", "token", "auth_key", "12345"} {
		if strings.Contains(strings.ToLower(text), banned) {
			t.Fatalf("notice leaks %q: %q", banned, text)
		}
	}
	if len(entities) == 0 || entities[0].GetLength() == 0 {
		t.Fatal("title is not marked")
	}
	empty, _ := loginSecurityNotice("  ", when)
	if !strings.Contains(empty, "未知设备") {
		t.Fatalf("empty device: %q", empty)
	}
}

func TestClaimLoginNoticeSkipsServiceUserAndRepeats(t *testing.T) {
	resetLoginNoticeClaims()
	t.Cleanup(resetLoginNoticeClaims)
	now := time.Unix(1_700_000_000, 0)
	if claimLoginNotice(777000, 1, now) {
		t.Fatal("service user must not be notified about itself")
	}
	if claimLoginNotice(0, 1, now) {
		t.Fatal("missing user")
	}
	if !claimLoginNotice(9, 100, now) {
		t.Fatal("first login")
	}
	if claimLoginNotice(9, 100, now.Add(30*time.Second)) {
		t.Fatal("same auth key inside two minutes")
	}
	if !claimLoginNotice(9, 200, now.Add(30*time.Second)) {
		t.Fatal("a different auth key is another device")
	}
	if !claimLoginNotice(9, 100, now.Add(3*time.Minute)) {
		t.Fatal("same key after the dedup window")
	}
}

func TestLoginDeviceLabelPrefersClient(t *testing.T) {
	if loginDeviceLabel(" android ", "ios") != "android" {
		t.Fatal("client")
	}
	if loginDeviceLabel("", " ios ") != "ios" {
		t.Fatal("langpack")
	}
	if loginDeviceLabel(" ", "") != "" {
		t.Fatal("empty")
	}
}

func TestLoginNoticeDevicePrefersModel(t *testing.T) {
	if loginNoticeDevice(" Samsung SM-S928B ", "android", "ios") != "Samsung SM-S928B" {
		t.Fatal("model")
	}
	if loginNoticeDevice("", "android", "ios") != "android" {
		t.Fatal("lang pack fallback")
	}
	if loginNoticeDevice(" ", "", "ios") != "ios" {
		t.Fatal("empty model")
	}
	text, _ := loginSecurityNotice(loginNoticeDevice("iPhone 16", "ios", "ios"), time.Date(2026, 11, 8, 0, 5, 0, 0, time.UTC))
	if !strings.Contains(text, "iPhone 16") || !strings.Contains(text, "2026-11-08 08:05") {
		t.Fatalf("composed notice: %q", text)
	}
	if strings.Contains(text, "验证码") {
		t.Fatal("composed notice has a code label")
	}
}
