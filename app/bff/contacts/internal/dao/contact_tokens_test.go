package dao

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/pkg/brand"
)

func TestContactTokenRoundTrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	b := &TokenBook{kv: newMemKV(), now: func() time.Time { return now }}
	token, expires, err := b.ExportContact(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if expires != int32(now.Add(contactShareTTL).Unix()) {
		t.Fatalf("expires %d", expires)
	}
	b.now = func() time.Time { return now.Add(time.Hour) }
	again, againExp, err := b.ExportContact(context.Background(), 7)
	if err != nil || again != token || againExp != expires {
		t.Fatalf("reuse %q %q exp %d %d %v", again, token, againExp, expires, err)
	}
	b.now = func() time.Time { return now }
	owner, err := b.LookupContact(context.Background(), token)
	if err != nil || owner != 7 {
		t.Fatalf("owner %d %v", owner, err)
	}
	if _, err := b.LookupContact(context.Background(), NormalizeContactToken("https://t.me/contact/"+token+"?free=x")); err != nil {
		t.Fatal(err)
	}
	b.now = func() time.Time { return now.Add(contactShareTTL + time.Second) }
	if _, err := b.LookupContact(context.Background(), token); err == nil {
		t.Fatal("expired token still resolved")
	}
}

func TestNoPaidTokenRevokeAndGrant(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	b := &TokenBook{kv: newMemKV(), now: func() time.Time { return now }}
	ctx := context.Background()
	perm, err := b.ExportNoPaid(ctx, 4, false, false)
	if err != nil || perm.Expires != 0 || perm.Token == "" {
		t.Fatalf("permanent %#v %v", perm, err)
	}
	same, err := b.ExportNoPaid(ctx, 4, false, false)
	if err != nil || same.Token != perm.Token {
		t.Fatalf("reuse %#v %v", same, err)
	}
	temp, err := b.ExportNoPaid(ctx, 4, false, true)
	if err != nil || !temp.Temporary || temp.Token == perm.Token {
		t.Fatalf("temporary %#v %v", temp, err)
	}
	if _, err := b.LookupNoPaid(ctx, temp.Token, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := b.LookupNoPaid(ctx, temp.Token, 9); err == nil {
		t.Fatal("token resolved for the wrong user")
	}
	next, err := b.ExportNoPaid(ctx, 4, true, false)
	if err != nil || next.Token == perm.Token {
		t.Fatalf("revoke %#v %v", next, err)
	}
	if _, err := b.LookupNoPaid(ctx, perm.Token, 4); err == nil {
		t.Fatal("revoked token still resolved")
	}
	if err := b.GrantNoPaid(ctx, 4, 8); err != nil {
		t.Fatal(err)
	}
	if !b.HasNoPaid(ctx, 4, 8) || b.HasNoPaid(ctx, 4, 3) {
		t.Fatal("exception set")
	}
	if got := NormalizeNoPaidToken("https://t.me/ada?free=" + next.Token + "&x=1"); got != next.Token {
		t.Fatalf("normalize %q", got)
	}
	if got := NoPaidLink("ada", "", next.Token); got != brand.LinkBase()+"/ada?free="+next.Token {
		t.Fatalf("username link %s", got)
	}
	if got := NoPaidLink("", "contacttok", next.Token); got != brand.LinkBase()+"/contact/contacttok?free="+next.Token {
		t.Fatalf("contact link %s", got)
	}
}

func TestGeneratedLinksUseBrandHost(t *testing.T) {
	t.Setenv("TELLOMI_LINK_BASE_URL", "")
	t.Setenv("TEAMGRAM_TME_HOST", "")
	if got := ContactLink("abc"); got != "https://tell.cc/contact/abc" {
		t.Fatalf("contact %s", got)
	}
	if got := NoPaidLink("ada", "", "tok"); got != "https://tell.cc/ada?free=tok" {
		t.Fatalf("username %s", got)
	}
	if got := NoPaidLink("bad name", "contacttok", "tok"); got != "https://tell.cc/contact/contacttok?free=tok" {
		t.Fatalf("fallback %s", got)
	}
	for _, got := range []string{ContactLink("abc"), NoPaidLink("ada", "", "tok")} {
		if strings.Contains(got, "t.me") || strings.Contains(got, "telegram.me") || strings.Contains(got, "telegram.dog") {
			t.Fatalf("generated link %s", got)
		}
	}
	if got := NormalizeContactToken(ContactLink("abc") + "?free=x"); got != "abc" {
		t.Fatalf("round trip %s", got)
	}
	for _, raw := range []string{
		"https://t.me/contact/abc?free=x",
		"http://t.me/contact/abc",
		"https://telegram.me/contact/abc&x",
		"tg://contact?token=abc",
		"https://tell.cc/contact/abc",
	} {
		if got := NormalizeContactToken(raw); got != "abc" {
			t.Fatalf("NormalizeContactToken(%q) = %q", raw, got)
		}
	}
	t.Setenv("TELLOMI_LINK_BASE_URL", "links.example")
	if got := ContactLink("abc"); got != "https://links.example/contact/abc" {
		t.Fatalf("env contact %s", got)
	}
	if got := NoPaidLink("ada", "", "tok"); got != "https://links.example/ada?free=tok" {
		t.Fatalf("env username %s", got)
	}
	if got := NormalizeContactToken("https://links.example/contact/abc?free=1"); got != "abc" {
		t.Fatalf("env normalize %s", got)
	}
	if got := NormalizeContactToken("https://t.me/contact/abc"); got != "abc" {
		t.Fatalf("legacy still imports under a custom base: %s", got)
	}
}
