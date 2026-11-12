package verify

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type memKV struct {
	mu        sync.Mutex
	m         map[string]string
	exp       map[string]time.Time
	now       func() time.Time
	expireErr error
}

func (m *memKV) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

func (m *memKV) sweep(key string) {
	if m.exp == nil || m.m == nil {
		return
	}
	exp, ok := m.exp[key]
	if !ok || exp.IsZero() {
		return
	}
	if !m.clock().Before(exp) {
		delete(m.m, key)
		delete(m.exp, key)
	}
}

func (m *memKV) GetCtx(ctx context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweep(key)
	return m.m[key], nil
}

func (m *memKV) SetnxExCtx(ctx context.Context, key, value string, seconds int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	m.sweep(key)
	if _, ok := m.m[key]; ok {
		return false, nil
	}
	m.m[key] = value
	if m.exp == nil {
		m.exp = map[string]time.Time{}
	}
	m.exp[key] = m.clock().Add(time.Duration(seconds) * time.Second)
	return true, nil
}

func (m *memKV) TtlCtx(ctx context.Context, key string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweep(key)
	if _, ok := m.m[key]; !ok {
		return -2, nil
	}
	exp, ok := m.exp[key]
	if !ok || exp.IsZero() {
		return -1, nil
	}
	remain := exp.Sub(m.clock())
	sec := int((remain + time.Second - 1) / time.Second)
	if sec < 1 {
		return 1, nil
	}
	return sec, nil
}

func (m *memKV) DecrCtx(ctx context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	n := 0
	if v := m.m[key]; v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			n = parsed - 1
		}
	}
	if n <= 0 {
		delete(m.m, key)
		delete(m.exp, key)
		return 0, nil
	}
	m.m[key] = strconv.Itoa(n)
	return int64(n), nil
}

func (m *memKV) SetexCtx(ctx context.Context, key, value string, seconds int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	m.m[key] = value
	return nil
}

func (m *memKV) IncrCtx(ctx context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	n := 1
	if v := m.m[key]; v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil {
			n = parsed + 1
		}
	}
	m.m[key] = strconv.Itoa(n)
	return int64(n), nil
}

func (m *memKV) EvalCtx(_ context.Context, _ string, phoneKey string, args ...any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.expireErr != nil {
		return nil, m.expireErr
	}
	if len(args) < 6 {
		return int64(-1), nil
	}
	phoneCap := argInt(args[0])
	ipCap := argInt(args[1])
	globalCap := argInt(args[2])
	ttl := argInt(args[3])
	ipKey, _ := args[4].(string)
	globalKey, _ := args[5].(string)
	cur := func(key string) int {
		if key == "" {
			return 0
		}
		m.sweep(key)
		n, _ := strconv.Atoi(m.m[key])
		return n
	}
	spend := func(key string, cap int) {
		if cap <= 0 || key == "" {
			return
		}
		n := cur(key) + 1
		if m.m == nil {
			m.m = map[string]string{}
		}
		m.m[key] = strconv.Itoa(n)
		if m.exp == nil {
			m.exp = map[string]time.Time{}
		}
		m.exp[key] = m.clock().Add(time.Duration(ttl) * time.Second)
	}
	if phoneCap > 0 && cur(phoneKey)+1 > phoneCap {
		return int64(1), nil
	}
	if ipCap > 0 && cur(ipKey)+1 > ipCap {
		return int64(2), nil
	}
	if globalCap > 0 && cur(globalKey)+1 > globalCap {
		return int64(3), nil
	}
	spend(phoneKey, phoneCap)
	spend(ipKey, ipCap)
	spend(globalKey, globalCap)
	return int64(0), nil
}

func argInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

func (m *memKV) ExpireCtx(_ context.Context, key string, seconds int) error {
	if m.expireErr != nil {
		return m.expireErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.exp == nil {
		m.exp = map[string]time.Time{}
	}
	m.exp[key] = m.clock().Add(time.Duration(seconds) * time.Second)
	return nil
}

func TestLimiterResendAndDailyCap(t *testing.T) {
	kv := &memKV{}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 2, DailyIP: 5})
	lim.now = func() time.Time { return time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC) }
	ctx := context.Background()

	if err := lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	err := lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4")
	le, ok := err.(*LimitError)
	if !ok || le.Daily || le.Retry <= 0 {
		t.Fatalf("cooldown: %#v", err)
	}

	kv.m = map[string]string{}
	if err := lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	delete(kv.m, "tellomi:code:cd:sms:8613800138000")
	if err := lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	delete(kv.m, "tellomi:code:cd:sms:8613800138000")
	err = lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4")
	le, ok = err.(*LimitError)
	if !ok || !le.Daily {
		t.Fatalf("daily: %#v", err)
	}
}

func TestDailyCapUsesShanghaiDayAndIsAtomic(t *testing.T) {
	kv := &memKV{}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 10, DailyIP: 0})
	lim.limits.ResendSeconds = 0
	// 2026-11-07 15:30 UTC is 23:30 in Asia/Shanghai, still the 7th.
	late := time.Date(2026, 11, 7, 15, 30, 0, 0, time.UTC)
	lim.now = func() time.Time { return late }
	ctx := context.Background()

	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- lim.AllowSend(ctx, "sms", "8613800138000", "8.8.8.8")
		}()
	}
	wg.Wait()
	close(errCh)
	okN, floodN := 0, 0
	for err := range errCh {
		if err == nil {
			okN++
			continue
		}
		le, ok := err.(*LimitError)
		if !ok || !le.Daily || le.Retry < 60 || le.Retry > 3600 {
			t.Fatalf("concurrent: %#v", err)
		}
		floodN++
	}
	if okN != 10 || floodN != 10 {
		t.Fatalf("allowed %d blocked %d", okN, floodN)
	}
	if _, exists := kv.m["tellomi:code:day:sms:ip:8.8.8.8:20261107"]; exists {
		t.Fatal("ip cap is off by default and must not be counted")
	}

	// 16:30 UTC is 00:30 the next Shanghai day. The counter starts over.
	lim.now = func() time.Time { return time.Date(2026, 11, 7, 16, 30, 0, 0, time.UTC) }
	if err := lim.AllowSend(ctx, "sms", "8613800138000", ""); err != nil {
		t.Fatal(err)
	}
	if kv.m["tellomi:code:day:sms:phone:8613800138000:20261108"] != "1" {
		t.Fatalf("next day %#v", kv.m)
	}
}

func TestShanghaiMidnightIsTheResetBoundary(t *testing.T) {
	kv := &memKV{}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 10})
	lim.limits.ResendSeconds = 0
	ctx := context.Background()
	phone := "8613800138000"
	// 15:59:59 UTC is 23:59:59 in Asia/Shanghai, still 7 November.
	lim.now = func() time.Time { return time.Date(2026, 11, 7, 15, 59, 59, 0, time.UTC) }
	for i := 0; i < 10; i++ {
		if err := lim.AllowSend(ctx, "sms", phone, ""); err != nil {
			t.Fatalf("send %d: %v", i+1, err)
		}
	}
	err := lim.AllowSend(ctx, "sms", phone, "")
	le, ok := err.(*LimitError)
	if !ok || !le.Daily || le.Retry != 1 {
		t.Fatalf("one second before midnight: %#v", err)
	}
	if kv.m["tellomi:code:day:sms:phone:"+phone+":20261107"] != "10" {
		t.Fatalf("old day counter %#v", kv.m)
	}
	// 16:00:00 UTC is 00:00:00 Asia/Shanghai on 8 November.
	lim.now = func() time.Time { return time.Date(2026, 11, 7, 16, 0, 0, 0, time.UTC) }
	if err := lim.AllowSend(ctx, "sms", phone, ""); err != nil {
		t.Fatal(err)
	}
	if kv.m["tellomi:code:day:sms:phone:"+phone+":20261108"] != "1" {
		t.Fatalf("midnight key %#v", kv.m)
	}
}

func TestPasteAndSIMFormsShareOneDailyKey(t *testing.T) {
	forms := []string{
		"+86 138 0013 8000",
		"8613800138000",
		"+86-138-0013-8000",
		"008613800138000",
		"+8613800138000",
	}
	var key string
	for _, form := range forms {
		_, digits, err := phonenumber.CheckPhoneNumberInvalid(form)
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if key == "" {
			key = digits
		}
		if digits != key {
			t.Fatalf("%s normalized to %s, want %s", form, digits, key)
		}
	}
	foreignForms := []string{"+1 415 555 2671", "0014155552671", "+44 20 7946 0958"}
	for _, form := range foreignForms {
		_, digits, err := phonenumber.CheckPhoneNumberInvalid(form)
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if err := RequireCountry(digits, false, nil); err != ErrInternationalDisabled {
			t.Fatalf("%s (%s) was accepted", form, digits)
		}
		if err := RequireCountry(digits, true, []string{"86"}); err != nil {
			t.Fatalf("allow international rejected %s: %v", form, err)
		}
	}

	kv := &memKV{}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 10})
	lim.limits.ResendSeconds = 0
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if err := lim.AllowSend(ctx, "sms", key, ""); err != nil {
			t.Fatal(err)
		}
	}
	err := lim.AllowSend(ctx, "sms", key, "")
	le, ok := err.(*LimitError)
	if !ok || !le.Daily {
		t.Fatalf("eleventh paste form: %#v", err)
	}
}

func TestEmailAndSMSQuotasAreSeparate(t *testing.T) {
	kv := &memKV{}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 1, DailyIP: 5})
	lim.now = func() time.Time { return time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC) }
	ctx := context.Background()

	if err := lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	if err := lim.AllowSend(ctx, "email", "user@tellomi.app", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
}

func TestCooldownIsOneWinnerUnderConcurrency(t *testing.T) {
	kv := &memKV{}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 10, DailyIP: 50})
	ctx := context.Background()
	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- lim.AllowSend(ctx, "sms", "8613800138000", "203.0.113.8")
		}()
	}
	wg.Wait()
	close(errCh)
	okN := 0
	for err := range errCh {
		if err == nil {
			okN++
			continue
		}
		le, ok := err.(*LimitError)
		if !ok || le.Daily || le.Retry != 60 {
			t.Fatalf("cooldown race: %#v", err)
		}
		if stringsContain(err.Error(), "8613800138000") || stringsContain(err.Error(), "203.0.113") {
			t.Fatalf("error names the target: %v", err)
		}
	}
	if okN != 1 {
		t.Fatalf("allowed %d sends", okN)
	}
}

func TestStoreErrorDoesNotSend(t *testing.T) {
	lim := NewLimiter(failKV{}, Limits{ResendSeconds: 60, DailyPhone: 10})
	err := lim.AllowSend(context.Background(), "sms", "8613800138000", "203.0.113.8")
	le, ok := err.(*LimitError)
	if !ok || le.Retry != 60 || stringsContain(err.Error(), "86138") {
		t.Fatalf("%#v", err)
	}
}

func TestGlobalAndIPBudgets(t *testing.T) {
	kv := &memKV{}
	lim := NewLimiter(kv, Limits{ResendSeconds: 0, DailyPhone: 10, DailyIP: 3, DailyGlobal: 4})
	ctx := context.Background()
	phones := []string{"8613800000001", "8613800000002", "8613800000003", "8613800000004", "8613800000005"}
	okN := 0
	for _, phone := range phones {
		err := lim.AllowSend(ctx, "sms", phone, "203.0.113.9")
		if err == nil {
			okN++
			continue
		}
		le, ok := err.(*LimitError)
		if !ok || !le.Daily {
			t.Fatalf("%s %#v", phone, err)
		}
	}
	if okN != 3 {
		t.Fatalf("ip cap allowed %d", okN)
	}
	kv.m = map[string]string{}
	okN = 0
	for i, phone := range phones {
		err := lim.AllowSend(ctx, "sms", phone, "198.51.100."+strconv.Itoa(i+1))
		if err == nil {
			okN++
		}
	}
	if okN != 4 {
		t.Fatalf("global cap allowed %d", okN)
	}
}

type failKV struct{}

func (failKV) GetCtx(context.Context, string) (string, error) { return "", context.DeadlineExceeded }
func (failKV) SetexCtx(context.Context, string, string, int) error {
	return context.DeadlineExceeded
}
func (failKV) IncrCtx(context.Context, string) (int64, error) {
	return 0, context.DeadlineExceeded
}
func (failKV) ExpireCtx(context.Context, string, int) error { return context.DeadlineExceeded }
func (failKV) SetnxExCtx(context.Context, string, string, int) (bool, error) {
	return false, context.DeadlineExceeded
}
func (failKV) TtlCtx(context.Context, string) (int, error) { return 0, context.DeadlineExceeded }
func (failKV) DecrCtx(context.Context, string) (int64, error) {
	return 0, context.DeadlineExceeded
}
func (failKV) EvalCtx(context.Context, string, string, ...any) (any, error) {
	return nil, context.DeadlineExceeded
}

func TestReviewRejectedRetryDoesNotExtendCooldown(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, shanghai)
	kv := &memKV{now: func() time.Time { return now }}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 10})
	lim.now = func() time.Time { return now }
	ctx := context.Background()
	if err := lim.AllowSend(ctx, "sms", "8613800138000", "203.0.113.8"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(59 * time.Second)
	err := lim.AllowSend(ctx, "sms", "8613800138000", "203.0.113.8")
	le, ok := err.(*LimitError)
	if !ok || le.Retry > 2 {
		t.Fatalf("retry extended the window: %#v", err)
	}
}

func TestReviewExpireFailureMustNotAdmit(t *testing.T) {
	kv := &memKV{expireErr: context.DeadlineExceeded}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 10})
	err := lim.AllowSend(context.Background(), "sms", "8613800138000", "203.0.113.8")
	le, ok := err.(*LimitError)
	if !ok || le == nil || le.Daily || le.Retry > storeRetry {
		t.Fatalf("expire failure admitted or locked the day: %#v", err)
	}
	kv.expireErr = nil
	delete(kv.m, "tellomi:code:cd:sms:8613800138000")
	if err = lim.AllowSend(context.Background(), "sms", "8613800138000", "203.0.113.8"); err != nil {
		t.Fatalf("recovered send: %v", err)
	}
}

func TestIndependentRedisScriptDoesNotSpendUnsentPhone(t *testing.T) {
	mr := miniredis.RunT(t)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	kv := redisKV{r: rds}
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, shanghai)
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 1, DailyIP: 1})
	lim.now = func() time.Time { return now }
	ctx := context.Background()
	if err = lim.AllowSend(ctx, "sms", "8613800000001", "203.0.113.8"); err != nil {
		t.Fatal(err)
	}
	err = lim.AllowSend(ctx, "sms", "8613800000002", "203.0.113.8")
	le, ok := err.(*LimitError)
	if !ok || !le.Daily {
		t.Fatalf("ip reject %#v", err)
	}
	day := now.Format("20060102")
	if v, _ := rds.GetCtx(ctx, "tellomi:code:day:sms:phone:8613800000002:"+day); v != "" {
		t.Fatalf("unsent phone %q", v)
	}
	_, _ = rds.DelCtx(ctx, "tellomi:code:cd:sms:8613800000002")
	if err = lim.AllowSend(ctx, "sms", "8613800000002", "203.0.113.9"); err != nil {
		t.Fatalf("new ip: %v", err)
	}
}

type redisKV struct{ r *redis.Redis }

func (s redisKV) GetCtx(ctx context.Context, key string) (string, error) {
	return s.r.GetCtx(ctx, key)
}
func (s redisKV) SetexCtx(ctx context.Context, key, value string, seconds int) error {
	return s.r.SetexCtx(ctx, key, value, seconds)
}
func (s redisKV) SetnxExCtx(ctx context.Context, key, value string, seconds int) (bool, error) {
	return s.r.SetnxExCtx(ctx, key, value, seconds)
}
func (s redisKV) TtlCtx(ctx context.Context, key string) (int, error) { return s.r.TtlCtx(ctx, key) }
func (s redisKV) IncrCtx(ctx context.Context, key string) (int64, error) {
	return s.r.IncrCtx(ctx, key)
}
func (s redisKV) DecrCtx(ctx context.Context, key string) (int64, error) {
	return s.r.DecrCtx(ctx, key)
}
func (s redisKV) ExpireCtx(ctx context.Context, key string, seconds int) error {
	return s.r.ExpireCtx(ctx, key, seconds)
}
func (s redisKV) EvalCtx(ctx context.Context, script string, key string, args ...any) (any, error) {
	return s.r.EvalCtx(ctx, script, []string{key}, args...)
}

func TestIndependentUnsentPhoneSurvivesRefundFailure(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, shanghai)
	base := &memKV{}
	kv := &decrFailKV{memKV: base}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 1, DailyIP: 1})
	lim.now = func() time.Time { return now }
	ctx := context.Background()
	if err := lim.AllowSend(ctx, "sms", "8613800000001", "203.0.113.8"); err != nil {
		t.Fatal(err)
	}
	err := lim.AllowSend(ctx, "sms", "8613800000002", "203.0.113.8")
	le, ok := err.(*LimitError)
	if !ok || !le.Daily {
		t.Fatalf("ip reject %#v", err)
	}
	day := now.In(shanghai).Format("20060102")
	if v := base.m["tellomi:code:day:sms:phone:8613800000002:"+day]; v != "" {
		t.Fatalf("unsent phone locked at %q", v)
	}
	delete(base.m, "tellomi:code:cd:sms:8613800000002")
	if err = lim.AllowSend(ctx, "sms", "8613800000002", "203.0.113.9"); err != nil {
		t.Fatalf("new ip still locked: %v", err)
	}
}

type decrFailKV struct{ *memKV }

func (d decrFailKV) DecrCtx(context.Context, string) (int64, error) {
	return 0, context.DeadlineExceeded
}

func TestReviewIPRejectionDoesNotSpendUnsentPhoneQuota(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, shanghai)
	kv := &memKV{}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 2, DailyIP: 1})
	lim.now = func() time.Time { return now }
	ctx := context.Background()
	if err := lim.AllowSend(ctx, "sms", "8613800000001", "203.0.113.8"); err != nil {
		t.Fatal(err)
	}
	err := lim.AllowSend(ctx, "sms", "8613800000002", "203.0.113.8")
	le, ok := err.(*LimitError)
	if !ok || !le.Daily {
		t.Fatalf("%#v", err)
	}
	day := now.In(shanghai).Format("20060102")
	if v := kv.m["tellomi:code:day:sms:phone:8613800000002:"+day]; v != "" {
		t.Fatalf("unsent phone quota spent %q", v)
	}
	if v := kv.m["tellomi:code:day:sms:phone:8613800000001:"+day]; v != "1" {
		t.Fatalf("sent phone quota %q", v)
	}
}

func stringsContain(s, part string) bool {
	return len(part) > 0 && len(s) >= len(part) && (s == part || len(s) > 0 && contains(s, part))
}

func contains(s, part string) bool {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
