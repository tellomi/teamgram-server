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
	SetnxExCtx(ctx context.Context, key, value string, seconds int) (bool, error)
	TtlCtx(ctx context.Context, key string) (int, error)
	IncrCtx(ctx context.Context, key string) (int64, error)
	DecrCtx(ctx context.Context, key string) (int64, error)
	ExpireCtx(ctx context.Context, key string, seconds int) error
	// EvalCtx matches the process cache: one script, one key, then args.
	// The phone key is KEYS[1]. IP and global keys travel as args and are
	// updated in the same script so a rejection writes nothing.
	EvalCtx(ctx context.Context, script string, key string, args ...any) (any, error)
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
	phoneKey := "tellomi:code:day:" + kind + ":phone:" + target + ":" + day
	ipKey := ""
	ipCap := 0
	if ip != "" && l.limits.DailyIP > 0 {
		ipKey = "tellomi:code:day:" + kind + ":ip:" + ip + ":" + day
		ipCap = l.limits.DailyIP
	}
	globalKey := ""
	globalCap := 0
	if l.limits.DailyGlobal > 0 {
		globalKey = "tellomi:code:day:" + kind + ":global:" + day
		globalCap = l.limits.DailyGlobal
	}
	// One script checks every cap and then increments the ones that apply.
	// A rejection writes nothing, so a failed refund cannot lock a number
	// that was never allowed to send.
	res, err := l.kv.EvalCtx(ctx, quotaScript, phoneKey,
		l.limits.DailyPhone, ipCap, globalCap, l.secondsUntilNextDay()+3600, ipKey, globalKey)
	if err != nil {
		return &LimitError{Retry: storeRetry}
	}
	switch evalCode(res) {
	case 0:
		return nil
	case 1, 2, 3:
		return &LimitError{Daily: true, Retry: l.secondsUntilNextDay()}
	default:
		return &LimitError{Retry: storeRetry}
	}
}

// quotaScript is atomic in Redis: either every permitted counter moves, or
// none do. Codes: 0 allow, 1 phone daily, 2 ip daily, 3 global daily.
const quotaScript = `
local phoneCap = tonumber(ARGV[1])
local ipCap = tonumber(ARGV[2])
local globalCap = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local ip = ARGV[5]
local glob = ARGV[6]
local phone = KEYS[1]
local function cur(key)
  if key == nil or key == '' then return 0 end
  return tonumber(redis.call('GET', key) or '0')
end
local function spend(key, cap)
  if cap > 0 and key ~= nil and key ~= '' then
    redis.call('INCR', key)
    redis.call('EXPIRE', key, ttl)
  end
end
if phoneCap > 0 and cur(phone) + 1 > phoneCap then return 1 end
if ipCap > 0 and cur(ip) + 1 > ipCap then return 2 end
if globalCap > 0 and cur(glob) + 1 > globalCap then return 3 end
spend(phone, phoneCap)
spend(ip, ipCap)
spend(glob, globalCap)
return 0
`

func evalCode(res any) int64 {
	switch v := res.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return -1
		}
		return n
	case []byte:
		return evalCode(string(v))
	case []any:
		if len(v) == 0 {
			return -1
		}
		return evalCode(v[0])
	default:
		return -1
	}
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
