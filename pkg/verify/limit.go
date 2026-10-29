package verify

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// KV is the subset of the process cache used for send quotas.
type KV interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetexCtx(ctx context.Context, key, value string, seconds int) error
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
	if limits.DailyIP <= 0 {
		limits.DailyIP = 30
	}
	return &Limiter{kv: kv, limits: limits, now: time.Now}
}

// AllowSend records one send. A non-nil error means the send must not happen.
func (l *Limiter) AllowSend(ctx context.Context, kind, target, ip string) error {
	if l == nil || l.kv == nil || target == "" {
		return nil
	}
	coolKey := "tellomi:code:cd:" + kind + ":" + target
	if v, err := l.kv.GetCtx(ctx, coolKey); err == nil && v != "" {
		retry := l.limits.ResendSeconds
		if n, conv := strconv.Atoi(v); conv == nil && n > 0 {
			retry = n
		}
		return &LimitError{Retry: retry}
	}

	day := l.now().UTC().Format("20060102")
	if err := l.bump(ctx, "tellomi:code:day:"+kind+":phone:"+target+":"+day, l.limits.DailyPhone); err != nil {
		return err
	}
	if ip != "" {
		if err := l.bump(ctx, "tellomi:code:day:"+kind+":ip:"+ip+":"+day, l.limits.DailyIP); err != nil {
			return err
		}
	}
	_ = l.kv.SetexCtx(ctx, coolKey, strconv.Itoa(l.limits.ResendSeconds), l.limits.ResendSeconds)
	return nil
}

func (l *Limiter) bump(ctx context.Context, key string, cap int) error {
	n := 1
	if v, err := l.kv.GetCtx(ctx, key); err == nil && v != "" {
		parsed, conv := strconv.Atoi(v)
		if conv == nil {
			n = parsed + 1
		}
	}
	if n > cap {
		return &LimitError{Daily: true}
	}
	return l.kv.SetexCtx(ctx, key, strconv.Itoa(n), 36*60*60)
}
