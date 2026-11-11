package verify

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// shanghai is China Standard Time. It has no daylight-saving shift.
var shanghai = time.FixedZone("Asia/Shanghai", 8*60*60)

// KV is the subset of the process cache used for send quotas.
// IncrCtx and ExpireCtx make the daily counter atomic across instances.
// Redis INCR is the production implementation. The key lives in Redis, so a
// process restart does not reset it.
type KV interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetexCtx(ctx context.Context, key, value string, seconds int) error
	SetnxExCtx(ctx context.Context, key, value string, seconds int) (bool, error)
	TtlCtx(ctx context.Context, key string) (int, error)
	IncrCtx(ctx context.Context, key string) (int64, error)
	DecrCtx(ctx context.Context, key string) (int64, error)
	ExpireCtx(ctx context.Context, key string, seconds int) error
}

// storeRetry is the wait after a cache error. It is not the rest of the day.
const storeRetry = 5

// LimitError is a send quota failure. Retry is the suggested wait in seconds.
type LimitError struct {
	Daily bool
	Retry int
}

func (e *LimitError) Error() string {
	if e.Daily {
		return "daily verification code limit"
	}
	if e.Retry > 0 {
		return fmt.Sprintf("resend available in %d seconds", e.Retry)
	}
	return "verification code rate limited"
}

// Limiter enforces a resend cooldown and per-phone / per-IP daily caps.
type Limiter struct {
	kv     KV
	limits Limits
	now    func() time.Time
}

func NewLimiter(kv KV, limits Limits) *Limiter {
	if limits.ResendSeconds <= 0 {
		limits.ResendSeconds = 60
	}
	if limits.DailyPhone <= 0 {
		limits.DailyPhone = 10
	}
	// DailyIP <= 0 leaves the per-IP cap off.
	return &Limiter{kv: kv, limits: limits, now: time.Now}
}

var (
	sharedMu    sync.RWMutex
	sharedSMS   *Limiter
	sharedEmail *Limiter
)

// UseSharedLimiters installs the process-wide SMS and email quotas.
// Send paths that are not the auth service (change phone, 2FA email) read these.
func UseSharedLimiters(sms, email *Limiter) {
	sharedMu.Lock()
	sharedSMS = sms
	sharedEmail = email
	sharedMu.Unlock()
}

// SharedSMS is the process-wide phone quota, or nil before startup.
func SharedSMS() *Limiter {
	sharedMu.RLock()
	defer sharedMu.RUnlock()
	return sharedSMS
}

// SharedEmail is the process-wide email quota, or nil before startup.
func SharedEmail() *Limiter {
	sharedMu.RLock()
	defer sharedMu.RUnlock()
	return sharedEmail
}

// AllowSend records one send. A non-nil error means the send must not happen.
// The cooldown is one INCR, so two processes cannot both observe an empty
// key and both call the provider. A store error is a cooldown, not a send.
func (l *Limiter) AllowSend(ctx context.Context, kind, target, ip string) error {
	if l == nil || l.kv == nil || target == "" {
		return nil
	}
	if l.limits.ResendSeconds > 0 {
		if err := l.cooldown(ctx, "tellomi:code:cd:"+kind+":"+target); err != nil {
			return err
		}
	}

	day := l.now().In(shanghai).Format("20060102")
	var held []string
	rollback := func() {
		for _, key := range held {
			_, _ = l.kv.DecrCtx(ctx, key)
		}
	}
	phoneKey := "tellomi:code:day:" + kind + ":phone:" + target + ":" + day
	if err := l.reserve(ctx, phoneKey, l.limits.DailyPhone, &held); err != nil {
		rollback()
		return err
	}
	// A per-IP cap stays off unless DailyIP is set above zero. Many phones
	// on one NAT address share it; one phone does not fill it alone when
	// the cap is above the per-phone cap. A rejection here returns the
	// phone reservation because the provider was not called.
	if ip != "" && l.limits.DailyIP > 0 {
		ipKey := "tellomi:code:day:" + kind + ":ip:" + ip + ":" + day
		if err := l.reserve(ctx, ipKey, l.limits.DailyIP, &held); err != nil {
			rollback()
			return err
		}
	}
	if l.limits.DailyGlobal > 0 {
		globalKey := "tellomi:code:day:" + kind + ":global:" + day
		if err := l.reserve(ctx, globalKey, l.limits.DailyGlobal, &held); err != nil {
			rollback()
			return err
		}
	}
	return nil
}

// cooldown places the resend key with its TTL in one SET NX EX. A later
// rejection reads the remaining TTL and does not refresh it. A store error
// does not admit the send.
func (l *Limiter) cooldown(ctx context.Context, key string) error {
	ok, err := l.kv.SetnxExCtx(ctx, key, "1", l.limits.ResendSeconds)
	if err != nil {
		return &LimitError{Retry: l.limits.ResendSeconds}
	}
	if ok {
		return nil
	}
	ttl, err := l.kv.TtlCtx(ctx, key)
	if err != nil {
		return &LimitError{Retry: l.limits.ResendSeconds}
	}
	if ttl < 0 {
		if err = l.kv.ExpireCtx(ctx, key, l.limits.ResendSeconds); err != nil {
			return &LimitError{Retry: storeRetry}
		}
		return &LimitError{Retry: l.limits.ResendSeconds}
	}
	if ttl < 1 {
		ttl = 1
	}
	if ttl > l.limits.ResendSeconds {
		ttl = l.limits.ResendSeconds
	}
	return &LimitError{Retry: ttl}
}

// reserve increments key. On success the key is appended to held so a later
// rejection can return it. The bucket that itself exceeds cap is not appended,
// so the rollback does not refund a real daily spend. A cache or TTL error
// undoes this increment and does not admit.
func (l *Limiter) reserve(ctx context.Context, key string, cap int, held *[]string) error {
	if cap <= 0 {
		return nil
	}
	n, err := l.kv.IncrCtx(ctx, key)
	if err != nil {
		return &LimitError{Retry: storeRetry}
	}
	if err = l.kv.ExpireCtx(ctx, key, l.secondsUntilNextDay()+3600); err != nil {
		_, _ = l.kv.DecrCtx(ctx, key)
		return &LimitError{Retry: storeRetry}
	}
	if int(n) > cap {
		return &LimitError{Daily: true, Retry: l.secondsUntilNextDay()}
	}
	*held = append(*held, key)
	return nil
}

func (l *Limiter) secondsUntilNextDay() int {
	now := l.now().In(shanghai)
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, shanghai)
	sec := int(next.Sub(now).Seconds())
	if sec < 1 {
		return 1
	}
	if sec > 86400 {
		return 86400
	}
	return sec
}
