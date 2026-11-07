package verify

import (
	"context"
	"fmt"
	"strconv"
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
	IncrCtx(ctx context.Context, key string) (int64, error)
	ExpireCtx(ctx context.Context, key string, seconds int) error
}

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
func (l *Limiter) AllowSend(ctx context.Context, kind, target, ip string) error {
	if l == nil || l.kv == nil || target == "" {
		return nil
	}
	coolKey := "tellomi:code:cd:" + kind + ":" + target
	if l.limits.ResendSeconds > 0 {
		if v, err := l.kv.GetCtx(ctx, coolKey); err == nil && v != "" {
			retry := l.limits.ResendSeconds
			if n, conv := strconv.Atoi(v); conv == nil && n > 0 {
				retry = n
			}
			return &LimitError{Retry: retry}
		}
	}

	day := l.now().In(shanghai).Format("20060102")
	if err := l.bump(ctx, "tellomi:code:day:"+kind+":phone:"+target+":"+day, l.limits.DailyPhone); err != nil {
		return err
	}
	// A per-IP cap stays off unless DailyIP is set above zero.
	if ip != "" && l.limits.DailyIP > 0 {
		if err := l.bump(ctx, "tellomi:code:day:"+kind+":ip:"+ip+":"+day, l.limits.DailyIP); err != nil {
			return err
		}
	}
	if l.limits.ResendSeconds > 0 {
		_ = l.kv.SetexCtx(ctx, coolKey, strconv.Itoa(l.limits.ResendSeconds), l.limits.ResendSeconds)
	}
	return nil
}

func (l *Limiter) bump(ctx context.Context, key string, cap int) error {
	if cap <= 0 {
		return nil
	}
	n, err := l.kv.IncrCtx(ctx, key)
	if err != nil {
		return err
	}
	// The TTL is the rest of the Shanghai day plus one hour, recomputed from
	// now, so a restart or a second instance does not extend the window.
	_ = l.kv.ExpireCtx(ctx, key, l.secondsUntilNextDay()+3600)
	if int(n) > cap {
		return &LimitError{Daily: true, Retry: l.secondsUntilNextDay()}
	}
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
