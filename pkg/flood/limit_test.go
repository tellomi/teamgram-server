package flood

import (
	"context"
	"testing"
	"time"
)

func TestClassOfAbuseMethods(t *testing.T) {
	cases := []struct {
		method string
		class  string
	}{
		{"auth_sendCode", classSendCode},
		{"auth_resendCode", classSendCode},
		{"auth_signIn", classSignIn},
		{"messages_sendMessage", classSend},
		{"messages_sendMedia", classSend},
		{"messages_sendMultiMedia", classSend},
		{"messages_forwardMessages", classSend},
		{"contacts_importContacts", classImport},
		{"contacts_resolveUsername", classResolve},
		{"contacts_search", classSearch},
		{"upload_saveFilePart", classUpload},
		{"upload_saveBigFilePart", classUpload},
		{"account_updateStatus", classStatus},
		{"account_updateProfile", classAccount},
		{"account_setPrivacy", classAccount},
		{"messages_reportSpam", classReport},
		{"account_reportPeer", classReport},
		{"updates_getState", classOther},
		{"messages_getWebPagePreview570D6F6F", classWebPreview},
		{"messages_getWebPagePreview8B68B0CC", classWebPreview},
		{"messages_getHistory", classOther},
		{"account_getAccountTTL", classOther},
		{"upload_getFile", classOther},
	}
	for _, tc := range cases {
		if got := classOf(tc.method); got != tc.class {
			t.Fatalf("%s -> %q, want %q", tc.method, got, tc.class)
		}
	}
}

func TestGlobalBurstThenWait(t *testing.T) {
	lim := New(NewMemory(), Config{
		Mode:  ModeEnforce,
		Rules: map[string]Rule{BucketGlobal: {Burst: 3, Rate: 0}},
	})
	now := time.Unix(1_700_000_000, 0)
	lim.now = func() time.Time { return now }
	in := Request{Method: "updates_getState", UserID: 7}
	for i := 0; i < 3; i++ {
		d := lim.Allow(context.Background(), in)
		if !d.Allowed || d.Limited {
			t.Fatalf("call %d %+v", i, d)
		}
	}
	d := lim.Allow(context.Background(), in)
	if d.Allowed || !d.Limited || d.WaitSec < 1 || d.Bucket != BucketGlobal {
		t.Fatalf("%+v", d)
	}
	// Another user is a different bucket.
	other := lim.Allow(context.Background(), Request{Method: "updates_getState", UserID: 8})
	if !other.Allowed {
		t.Fatalf("other user %+v", other)
	}
	// No user id falls back to the auth key.
	auth := Request{Method: "updates_getState", AuthKey: 99}
	if !lim.Allow(context.Background(), auth).Allowed {
		t.Fatal("auth key")
	}
}

func TestLogModeDoesNotReject(t *testing.T) {
	lim := New(NewMemory(), Config{
		Mode:  ModeLog,
		Rules: map[string]Rule{BucketGlobal: {Burst: 1, Rate: 0}},
	})
	in := Request{Method: "updates_getState", UserID: 1}
	_ = lim.Allow(context.Background(), in)
	d := lim.Allow(context.Background(), in)
	if !d.Allowed || !d.Limited || d.WaitSec < 1 {
		t.Fatalf("log mode %+v", d)
	}
}

func TestOffMode(t *testing.T) {
	lim := New(NewMemory(), Config{Mode: ModeOff, Rules: map[string]Rule{BucketGlobal: {Burst: 1, Rate: 0}}})
	in := Request{Method: "updates_getState", UserID: 1}
	for i := 0; i < 5; i++ {
		if d := lim.Allow(context.Background(), in); !d.Allowed || d.Limited {
			t.Fatalf("%+v", d)
		}
	}
}

func TestSendCodePhoneAndIPAreSeparate(t *testing.T) {
	lim := New(NewMemory(), Config{
		Mode: ModeEnforce,
		Rules: map[string]Rule{
			BucketGlobal:        {Burst: 100, Rate: 0},
			BucketSendCodePhone: {Burst: 2, Rate: 0},
			BucketSendCodeIP:    {Burst: 100, Rate: 0},
		},
	})
	a := Request{Method: "auth_sendCode", AuthKey: 1, IP: "203.0.113.5", Phone: "8613800000001"}
	b := Request{Method: "auth_sendCode", AuthKey: 2, IP: "203.0.113.5", Phone: "8613800000002"}
	if !lim.Allow(context.Background(), a).Allowed || !lim.Allow(context.Background(), a).Allowed {
		t.Fatal("phone burst")
	}
	if d := lim.Allow(context.Background(), a); d.Allowed || d.Bucket != BucketSendCodePhone || d.Scope != "phone" {
		t.Fatalf("phone flood %+v", d)
	}
	if d := lim.Allow(context.Background(), b); !d.Allowed {
		t.Fatalf("other phone %+v", d)
	}
}

func TestUploadSkipsGlobal(t *testing.T) {
	lim := New(NewMemory(), Config{
		Mode: ModeEnforce,
		Rules: map[string]Rule{
			BucketGlobal: {Burst: 1, Rate: 0},
			BucketUpload: {Burst: 3, Rate: 0},
		},
	})
	in := Request{Method: "upload_saveFilePart", UserID: 4}
	for i := 0; i < 3; i++ {
		if d := lim.Allow(context.Background(), in); !d.Allowed {
			t.Fatalf("part %d %+v", i, d)
		}
	}
	if d := lim.Allow(context.Background(), in); d.Allowed || d.Bucket != BucketUpload {
		t.Fatalf("upload %+v", d)
	}
	// The global bucket was not spent, so a following read still fits.
	if d := lim.Allow(context.Background(), Request{Method: "updates_getState", UserID: 4}); !d.Allowed {
		t.Fatalf("global %+v", d)
	}
}

func TestRefill(t *testing.T) {
	lim := New(NewMemory(), Config{
		Mode:  ModeEnforce,
		Rules: map[string]Rule{BucketContactsSearch: {Burst: 1, Rate: 1}, BucketGlobal: {Burst: 100, Rate: 100}},
	})
	now := time.Unix(1_700_000_000, 0)
	lim.now = func() time.Time { return now }
	in := Request{Method: "contacts_search", UserID: 3}
	if !lim.Allow(context.Background(), in).Allowed {
		t.Fatal("first")
	}
	if lim.Allow(context.Background(), in).Allowed {
		t.Fatal("second should wait")
	}
	now = now.Add(time.Second)
	if d := lim.Allow(context.Background(), in); !d.Allowed {
		t.Fatalf("after refill %+v", d)
	}
}

func TestParseLimits(t *testing.T) {
	t.Setenv("TELLOMI_FLOOD_MODE", "log")
	t.Setenv("TELLOMI_FLOOD_LIMITS", "send:4:1.5, nope, global:9:x, contacts_search:12:3")
	cfg := ConfigFromEnv()
	if cfg.Mode != ModeLog {
		t.Fatalf("mode %s", cfg.Mode)
	}
	if cfg.Rules[BucketSend].Burst != 4 || cfg.Rules[BucketSend].Rate != 1.5 {
		t.Fatalf("send %+v", cfg.Rules[BucketSend])
	}
	if cfg.Rules[BucketContactsSearch].Burst != 12 {
		t.Fatalf("search %+v", cfg.Rules[BucketContactsSearch])
	}
	if cfg.Rules[BucketGlobal].Burst != defaultRules()[BucketGlobal].Burst {
		t.Fatal("bad global override applied")
	}
}

func TestStartupBurstFits(t *testing.T) {
	lim := New(NewMemory(), DefaultConfig())
	in := Request{Method: "updates_getState", UserID: 42}
	for i := 0; i < 120; i++ {
		if d := lim.Allow(context.Background(), in); !d.Allowed {
			t.Fatalf("startup call %d %+v", i, d)
		}
	}
	send := Request{Method: "messages_sendMessage", UserID: 42}
	for i := 0; i < 60; i++ {
		if d := lim.Allow(context.Background(), send); !d.Allowed {
			t.Fatalf("send %d %+v", i, d)
		}
	}
}
