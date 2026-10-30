package dao

import (
	"context"
	"testing"
	"time"
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
	if got := NoPaidLink("ada", "", next.Token); got != "https://t.me/ada?free="+next.Token {
		t.Fatalf("username link %s", got)
	}
	if got := NoPaidLink("", "contacttok", next.Token); got != "https://t.me/contact/contacttok?free="+next.Token {
		t.Fatalf("contact link %s", got)
	}
}
