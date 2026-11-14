package flood

import (
	"context"
	"strconv"
	"sync"
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

type errStore struct{ err error }

func (e errStore) Take(context.Context, string, Rule, time.Time) (bool, time.Duration, error) {
	return false, 0, e.err
}

func TestPerBucketModeRollback(t *testing.T) {
	lim := New(NewMemory(), Config{
		Mode: ModeLog,
		Modes: map[string]string{
			BucketSend: BucketModeEnforce(),
		},
		Rules: map[string]Rule{
			BucketGlobal: {Burst: 1, Rate: 0},
			BucketSend:   {Burst: 1, Rate: 0},
		},
	})
	read := Request{Method: "updates_getState", UserID: 1}
	_ = lim.Allow(context.Background(), read)
	d := lim.Allow(context.Background(), read)
	if !d.Allowed || !d.Limited {
		t.Fatalf("read stayed log %+v", d)
	}
	send := Request{Method: "messages_sendMessage", UserID: 1}
	if !lim.Allow(context.Background(), send).Allowed {
		t.Fatal("first send")
	}
	d = lim.Allow(context.Background(), send)
	if d.Allowed || !d.Limited || d.Bucket != BucketSend || d.WaitSec < 1 {
		t.Fatalf("send enforce %+v", d)
	}
}

func BucketModeEnforce() string { return ModeEnforce }

func TestRedisOutageBoundsHighCostOnly(t *testing.T) {
	lim := New(errStore{err: context.DeadlineExceeded}, Config{
		Mode: ModeEnforce,
		Rules: map[string]Rule{
			BucketGlobal: {Burst: 1, Rate: 0},
			BucketSend:   {Burst: 1, Rate: 0},
		},
	})
	read := Request{Method: "updates_getState", UserID: 9}
	for i := 0; i < 5; i++ {
		if d := lim.Allow(context.Background(), read); !d.Allowed || d.Limited {
			t.Fatalf("read %d %+v", i, d)
		}
	}
	send := Request{Method: "messages_sendMessage", UserID: 9}
	if !lim.Allow(context.Background(), send).Allowed {
		t.Fatal("local fallback first send")
	}
	d := lim.Allow(context.Background(), send)
	if d.Allowed || d.Bucket != BucketSend || d.WaitSec < 1 {
		t.Fatalf("local fallback bound %+v", d)
	}
}

func TestReviewOutageRejectsDoNotGrowMemoryForever(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	lim := New(errStore{err: context.DeadlineExceeded}, Config{
		Mode: ModeEnforce,
		Rules: map[string]Rule{
			BucketSendCodePhone: {Burst: 5, Rate: 0},
			BucketSendCodeIP:    {Burst: 1, Rate: 0},
		},
	})
	lim.now = func() time.Time { return now }
	for i := 0; i < 1000; i++ {
		d := lim.Allow(context.Background(), Request{
			Method: "auth_sendCode",
			Phone:  "100000" + strconv.Itoa(i),
			IP:     "203.0.113.8",
		})
		if i == 0 && !d.Allowed {
			t.Fatalf("first send %+v", d)
		}
		if i > 0 && d.Allowed {
			t.Fatalf("ip cap did not hold at %d %+v", i, d)
		}
	}
	if n := lim.local.Len(); n > 4 {
		t.Fatalf("rejected phones kept %d local keys", n)
	}
	before := lim.local.Len()
	lim.local.maxKeys = 2
	lim.cfg.Rules[BucketSend] = Rule{Burst: 1, Rate: 0}
	for i := 0; i < 20; i++ {
		_ = lim.Allow(context.Background(), Request{
			Method: "messages_sendMessage",
			UserID: int64(1000 + i),
		})
	}
	if n := lim.local.Len() - before; n > 6 {
		t.Fatalf("overflow grew by %d", n)
	}
	now = now.Add(24 * time.Hour)
	_ = lim.Allow(context.Background(), Request{Method: "auth_sendCode", Phone: "1999", IP: "203.0.113.9"})
	if n := lim.local.Len(); n > 4 {
		t.Fatalf("entries survived 24h: %d", n)
	}
}

func TestIndependentRejectedPhonesDoNotFillLocalTable(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0)
	lim := New(errStore{err: context.DeadlineExceeded}, Config{
		Mode: ModeEnforce,
		Rules: map[string]Rule{
			BucketSendCodePhone: {Burst: 5, Rate: 0},
			BucketSendCodeIP:    {Burst: 1, Rate: 0},
		},
	})
	lim.now = func() time.Time { return now }
	for i := 0; i < 4106; i++ {
		d := lim.Allow(ctx, Request{
			Method: "auth_sendCode",
			Phone:  strconv.Itoa(100000 + i),
			IP:     "203.0.113.8",
		})
		if i == 0 && !d.Allowed {
			t.Fatalf("first phone %+v", d)
		}
		if i > 0 && d.Allowed {
			t.Fatalf("phone %d allowed %+v", i, d)
		}
	}
	if n := lim.local.Len(); n > 4 {
		t.Fatalf("rejected phones left %d keys", n)
	}
	allows := 0
	for i := 0; i < 3; i++ {
		d := lim.Allow(ctx, Request{
			Method: "auth_sendCode",
			Phone:  strconv.Itoa(900000 + i),
			IP:     "198.51.100." + strconv.Itoa(i+1),
		})
		if d.Allowed {
			allows++
		}
	}
	if allows != 3 {
		t.Fatalf("fresh phone and ip allowed %d", allows)
	}
}

// blockIPStore fails every Redis take. The first Take of blockKey, after
// Arm, waits until Release so a second request can spend the same phone.
type blockIPStore struct {
	blockKey string
	armed    bool
	entered  chan struct{}
	release  chan struct{}
	mu       sync.Mutex
	held     bool
}

func (b *blockIPStore) Arm() {
	b.mu.Lock()
	b.armed = true
	b.mu.Unlock()
}

func (b *blockIPStore) Take(_ context.Context, key string, _ Rule, _ time.Time) (bool, time.Duration, error) {
	b.mu.Lock()
	hold := b.armed && !b.held && key == b.blockKey
	if hold {
		b.held = true
	}
	b.mu.Unlock()
	if hold {
		close(b.entered)
		<-b.release
	}
	return false, 0, context.DeadlineExceeded
}

func TestIndependentRejectDoesNotDropConcurrentPhoneSpend(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0)
	ipKey := "flood:" + BucketSendCodeIP + ":ip:203.0.113.8"
	st := &blockIPStore{
		blockKey: ipKey,
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	lim := New(st, Config{
		Mode: ModeEnforce,
		Rules: map[string]Rule{
			BucketSendCodePhone: {Burst: 2, Rate: 0},
			BucketSendCodeIP:    {Burst: 1, Rate: 0},
		},
	})
	lim.now = func() time.Time { return now }
	if d := lim.Allow(ctx, Request{Method: "auth_sendCode", Phone: "100", IP: "203.0.113.8"}); !d.Allowed {
		t.Fatalf("seed %+v", d)
	}
	st.Arm()

	var rejected Decision
	done := make(chan struct{})
	go func() {
		rejected = lim.Allow(ctx, Request{Method: "auth_sendCode", Phone: "200", IP: "203.0.113.8"})
		close(done)
	}()
	select {
	case <-st.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("rejected request did not reach the IP check")
	}
	// Same phone, a new IP. This spend must survive the first request's
	// rejection. Dropping the whole phone bucket here would restore a full
	// burst and allow more than 2 sends.
	if d := lim.Allow(ctx, Request{Method: "auth_sendCode", Phone: "200", IP: "198.51.100.1"}); !d.Allowed {
		t.Fatalf("concurrent phone spend %+v", d)
	}
	close(st.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("rejected request did not finish")
	}
	if rejected.Allowed {
		t.Fatalf("limited IP was allowed %+v", rejected)
	}
	third := lim.Allow(ctx, Request{Method: "auth_sendCode", Phone: "200", IP: "198.51.100.2"})
	fourth := lim.Allow(ctx, Request{Method: "auth_sendCode", Phone: "200", IP: "198.51.100.3"})
	if !third.Allowed || fourth.Allowed {
		t.Fatalf("burst 2 after the raced rejection: third=%v fourth=%v", third.Allowed, fourth.Allowed)
	}
}

func TestIndependentRejectedRequestDoesNotResetPhoneBucket(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0)
	lim := New(errStore{err: context.DeadlineExceeded}, Config{
		Mode: ModeEnforce,
		Rules: map[string]Rule{
			BucketSendCodePhone: {Burst: 2, Rate: 0},
			BucketSendCodeIP:    {Burst: 1, Rate: 0},
		},
	})
	lim.now = func() time.Time { return now }
	// Fixed order: two different phones share an IP cap of one, then the
	// first phone is asked again. A rejection must not delete that phone's
	// remaining tokens.
	steps := []Request{
		{Method: "auth_sendCode", Phone: "8613800000001", IP: "203.0.113.1"},
		{Method: "auth_sendCode", Phone: "8613800000001", IP: "203.0.113.1"},
		{Method: "auth_sendCode", Phone: "8613800000001", IP: "203.0.113.2"},
		{Method: "auth_sendCode", Phone: "8613800000001", IP: "203.0.113.3"},
		{Method: "auth_sendCode", Phone: "8613800000001", IP: "203.0.113.4"},
	}
	allows := 0
	for i, req := range steps {
		d := lim.Allow(ctx, req)
		t.Logf("step %d allowed=%v bucket=%s wait=%d", i, d.Allowed, d.Bucket, d.WaitSec)
		if d.Allowed {
			allows++
		}
	}
	if allows != 2 {
		t.Fatalf("phone sent %d times with burst 2", allows)
	}
}

func TestIndependentOverflowDoesNotRefillAcrossRules(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0)
	mem := NewMemory()
	mem.maxKeys = 1
	slow := Rule{Burst: 1, Rate: 0.01}
	fast := Rule{Burst: 100, Rate: 100}
	if ok, _, err := mem.Take(ctx, "flood:upload_part:user:1", Rule{Burst: 1, Rate: 0}, now); err != nil || !ok {
		t.Fatalf("saturate %v %v", ok, err)
	}
	if ok, _, err := mem.Take(ctx, "flood:send_code_phone:phone:8613800000001", slow, now); err != nil || !ok {
		t.Fatalf("first code %v %v", ok, err)
	}
	now = now.Add(time.Second)
	if _, _, err := mem.Take(ctx, "flood:send:user:9", fast, now); err != nil {
		t.Fatal(err)
	}
	ok, wait, err := mem.Take(ctx, "flood:send_code_phone:phone:8613800000002", slow, now)
	if err != nil || ok {
		t.Fatalf("fast rule refilled send-code ok=%v wait=%s err=%v", ok, wait, err)
	}
	if wait < 90*time.Second {
		t.Fatalf("send-code wait %s, want about 100s", wait)
	}

	lim := New(errStore{err: context.DeadlineExceeded}, Config{
		Mode: ModeEnforce,
		Rules: map[string]Rule{
			BucketSendCodePhone: slow,
			BucketSend:          fast,
		},
	})
	lim.local.maxKeys = 1
	lim.now = func() time.Time { return now }
	if _, _, err := lim.local.Take(ctx, "flood:upload_part:user:1", Rule{Burst: 1, Rate: 0}, now); err != nil {
		t.Fatal(err)
	}
	if d := lim.Allow(ctx, Request{Method: "auth_sendCode", Phone: "8613800000001"}); !d.Allowed {
		t.Fatalf("limiter first code %+v", d)
	}
	now = now.Add(time.Second)
	_ = lim.Allow(ctx, Request{Method: "messages_sendMessage", UserID: 9})
	d := lim.Allow(ctx, Request{Method: "auth_sendCode", Phone: "8613800000002"})
	if d.Allowed || d.WaitSec < 90 {
		t.Fatalf("limiter overflow %+v", d)
	}
}

func TestJitterStaysInsideTenPercent(t *testing.T) {
	a := jitterWait(BucketSend, "1", 100)
	b := jitterWait(BucketSend, "2", 100)
	if a < 100 || a > 110 || b < 100 || b > 110 {
		t.Fatalf("jitter %d %d", a, b)
	}
	if jitterWait(BucketSend, "1", 100) != a {
		t.Fatal("jitter changed for the same subject")
	}
}

func TestBucketModeEnv(t *testing.T) {
	t.Setenv("TELLOMI_FLOOD_MODE", "log")
	t.Setenv("TELLOMI_FLOOD_BUCKET_MODE", "send:enforce, global:off, nope:enforce, upload:maybe")
	cfg := ConfigFromEnv()
	if cfg.Mode != ModeLog || cfg.Modes[BucketSend] != ModeEnforce || cfg.Modes[BucketGlobal] != ModeOff {
		t.Fatalf("%s %#v", cfg.Mode, cfg.Modes)
	}
	if _, ok := cfg.Modes["nope"]; ok {
		t.Fatal("unknown bucket mode applied")
	}
	if _, ok := cfg.Modes[BucketUpload]; ok {
		t.Fatal("bad mode applied")
	}
}

func TestGradualModeFromEnv(t *testing.T) {
	t.Setenv("TELLOMI_FLOOD_MODE", "gradual")
	t.Setenv("TELLOMI_FLOOD_BUCKET_MODE", "send:log, contacts_search:enforce, nope:enforce")
	cfg := ConfigFromEnv()
	if cfg.Mode != ModeGradual {
		t.Fatalf("mode %s", cfg.Mode)
	}
	if cfg.Modes[BucketSend] != ModeLog {
		t.Fatalf("send override %+v", cfg.Modes)
	}
	if cfg.Modes[BucketContactsSearch] != ModeEnforce {
		t.Fatalf("search override %+v", cfg.Modes)
	}
	if _, ok := cfg.Modes["nope"]; ok {
		t.Fatal("unknown bucket")
	}
	lim := New(NewMemory(), cfg)
	if lim.modeOf(BucketSend) != ModeLog {
		t.Fatalf("send mode %s", lim.modeOf(BucketSend))
	}
	if lim.modeOf(BucketContactsSearch) != ModeEnforce {
		t.Fatalf("search mode %s", lim.modeOf(BucketContactsSearch))
	}
	if lim.modeOf(BucketGlobal) != ModeLog {
		t.Fatalf("global mode %s", lim.modeOf(BucketGlobal))
	}
	if lim.modeOf(BucketUpload) != ModeEnforce {
		t.Fatalf("upload mode %s", lim.modeOf(BucketUpload))
	}
}

func TestGradualEnforcesHighCostLogsOthers(t *testing.T) {
	lim := New(NewMemory(), Config{
		Mode: ModeGradual,
		Rules: map[string]Rule{
			// Global is also charged on these methods; keep it large so the
			// per-bucket mode is what the assertion sees.
			BucketGlobal:         {Burst: 100, Rate: 0},
			BucketSend:           {Burst: 1, Rate: 0},
			BucketContactsSearch: {Burst: 1, Rate: 0},
			BucketAccountStatus:  {Burst: 1, Rate: 0},
			BucketSendCodePhone:  {Burst: 1, Rate: 0},
			BucketSendCodeIP:     {Burst: 1, Rate: 0},
		},
	})
	glob := New(NewMemory(), Config{
		Mode:  ModeGradual,
		Rules: map[string]Rule{BucketGlobal: {Burst: 1, Rate: 0}},
	})
	read := Request{Method: "updates_getState", UserID: 1}
	if !glob.Allow(context.Background(), read).Allowed {
		t.Fatal("first read")
	}
	d := glob.Allow(context.Background(), read)
	if !d.Allowed || !d.Limited || d.Bucket != BucketGlobal {
		t.Fatalf("global stayed log %+v", d)
	}
	search := Request{Method: "contacts_search", UserID: 1}
	if !lim.Allow(context.Background(), search).Allowed {
		t.Fatal("first search")
	}
	d = lim.Allow(context.Background(), search)
	if !d.Allowed || !d.Limited || d.Bucket != BucketContactsSearch {
		t.Fatalf("search stayed log %+v", d)
	}
	status := Request{Method: "account_updateStatus", UserID: 1}
	if !lim.Allow(context.Background(), status).Allowed {
		t.Fatal("first status")
	}
	d = lim.Allow(context.Background(), status)
	if !d.Allowed || !d.Limited || d.Bucket != BucketAccountStatus {
		t.Fatalf("status stayed log %+v", d)
	}
	send := Request{Method: "messages_sendMessage", UserID: 1}
	if !lim.Allow(context.Background(), send).Allowed {
		t.Fatal("first send")
	}
	d = lim.Allow(context.Background(), send)
	if d.Allowed || !d.Limited || d.Bucket != BucketSend || d.WaitSec < 1 {
		t.Fatalf("send enforce %+v", d)
	}
	code := Request{Method: "auth_sendCode", Phone: "8613800000000", IP: "203.0.113.9"}
	if !lim.Allow(context.Background(), code).Allowed {
		t.Fatal("first sendCode")
	}
	d = lim.Allow(context.Background(), code)
	if d.Allowed || !d.Limited || (d.Bucket != BucketSendCodePhone && d.Bucket != BucketSendCodeIP) || d.WaitSec < 1 {
		t.Fatalf("sendCode enforce %+v", d)
	}
}

func TestGradualBucketModeOverride(t *testing.T) {
	lim := New(NewMemory(), Config{
		Mode: ModeGradual,
		Modes: map[string]string{
			BucketSend:   ModeLog,
			BucketGlobal: ModeEnforce,
		},
		Rules: map[string]Rule{
			BucketGlobal: {Burst: 100, Rate: 0},
			BucketSend:   {Burst: 1, Rate: 0},
		},
	})
	send := Request{Method: "messages_sendMessage", UserID: 2}
	if !lim.Allow(context.Background(), send).Allowed {
		t.Fatal("first send")
	}
	d := lim.Allow(context.Background(), send)
	if !d.Allowed || !d.Limited || d.Bucket != BucketSend {
		t.Fatalf("send override stayed log %+v", d)
	}

	lim2 := New(NewMemory(), Config{
		Mode: ModeGradual,
		Modes: map[string]string{
			BucketGlobal: ModeEnforce,
		},
		Rules: map[string]Rule{
			BucketGlobal: {Burst: 1, Rate: 0},
		},
	})
	read := Request{Method: "updates_getState", UserID: 2}
	if !lim2.Allow(context.Background(), read).Allowed {
		t.Fatal("first read")
	}
	d = lim2.Allow(context.Background(), read)
	if d.Allowed || !d.Limited || d.Bucket != BucketGlobal || d.WaitSec < 1 {
		t.Fatalf("global override enforce %+v", d)
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
