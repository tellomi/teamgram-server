// Package flood limits abuse-prone MTProto methods per user, auth key,
// phone, and client IP. The counters live in Redis so every session
// process shares them. A normal official client, including the burst at
// startup and while a chat syncs, stays under the defaults.
package flood

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	ModeOff     = "off"
	ModeLog     = "log"
	ModeEnforce = "enforce"

	BucketGlobal         = "global"
	BucketSend           = "send"
	BucketSendCodePhone  = "send_code_phone"
	BucketSendCodeIP     = "send_code_ip"
	BucketSignInPhone    = "sign_in_phone"
	BucketSignInIP       = "sign_in_ip"
	BucketImportContacts = "import_contacts"
	BucketResolve        = "resolve_username"
	BucketContactsSearch = "contacts_search"
	BucketUpload         = "upload_part"
	BucketAccountStatus  = "account_status"
	BucketAccountUpdate  = "account_update"
	BucketReport         = "report"
	BucketWebPreview     = "web_preview"
)

// Rule is a token bucket. Burst is the bucket size. Rate is tokens added
// per second. Rate 0 does not refill.
type Rule struct {
	Burst int64
	Rate  float64
}

// Config is the process-wide limiter. Mode is off, log, or enforce.
type Config struct {
	Mode  string
	Rules map[string]Rule
}

// DefaultConfig is what production runs. The numbers are above what an
// official client does at startup, while scrolling, and while uploading,
// and below an open loop against the abuse-prone methods.
func DefaultConfig() Config {
	return Config{
		Mode:  ModeEnforce,
		Rules: defaultRules(),
	}
}

func defaultRules() map[string]Rule {
	return map[string]Rule{
		// Everything except upload part writes. Startup is about a hundred
		// RPCs; a file download can be parallel. 200/s after a burst of
		// 2000 still stops a tight loop.
		BucketGlobal: {Burst: 2000, Rate: 200},
		// messages.send* and forwardMessages. A history test and a sticker
		// burst fit in 80. Five a second is faster than a person.
		BucketSend: {Burst: 80, Rate: 5},
		// auth.sendCode per phone. The SMS resend cooldown is 60s and the
		// daily cap is 10, so a person never holds the bucket empty. A few
		// immediate retries still fit.
		BucketSendCodePhone: {Burst: 8, Rate: 0.05},
		// Same method per client IP. Carrier NAT puts many people on one
		// address, and the e2e suite logs in from one address, so this stays
		// loose. The phone bucket is the tight one.
		BucketSendCodeIP: {Burst: 200, Rate: 2},
		// auth.signIn per phone. Wrong codes already stop at 5. This is the
		// backstop for a loop that never reaches that counter.
		BucketSignInPhone: {Burst: 20, Rate: 0.2},
		BucketSignInIP:    {Burst: 200, Rate: 2},
		// contacts.importContacts. A large address book is a handful of
		// chunks at startup, not a loop. After that burst the bucket refills
		// slowly, so a scanner cannot walk phone numbers.
		BucketImportContacts: {Burst: 20, Rate: 0.05},
		// contacts.resolveUsername. Opening profiles and links, not a scan
		// of every name.
		BucketResolve: {Burst: 80, Rate: 5},
		// contacts.search fires on each keystroke. A fast typist is a few
		// queries a second.
		BucketContactsSearch: {Burst: 40, Rate: 8},
		// upload.saveFilePart / saveBigFilePart. A large file is hundreds of
		// parts, sent in parallel. This bucket is not also charged to global.
		BucketUpload: {Burst: 2000, Rate: 400},
		// account.updateStatus on foreground, background, and the periodic
		// online ping.
		BucketAccountStatus: {Burst: 30, Rate: 1},
		// Other account mutations: profile, privacy, notify settings.
		BucketAccountUpdate: {Burst: 30, Rate: 0.5},
		// Reports. A multi-select report fits in the burst. The report
		// service keeps its own hourly cap on top of this.
		BucketReport: {Burst: 15, Rate: 0.05},
		// messages.getWebPagePreview while a link is typed. A few calls as
		// the draft changes fit in the burst. One a second after that is
		// faster than a person and stops a loop that would fetch the web.
		BucketWebPreview: {Burst: 20, Rate: 1},
	}
}

// ConfigFromEnv reads TELLOMI_FLOOD_MODE and TELLOMI_FLOOD_LIMITS.
// MODE is enforce, log, or off. Empty is enforce.
// LIMITS is name:burst:rate,name:burst:rate and overrides those buckets.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TELLOMI_FLOOD_MODE"))) {
	case ModeOff:
		cfg.Mode = ModeOff
	case ModeLog:
		cfg.Mode = ModeLog
	case "", ModeEnforce:
		cfg.Mode = ModeEnforce
	default:
		cfg.Mode = ModeEnforce
	}
	for _, part := range strings.Split(os.Getenv("TELLOMI_FLOOD_LIMITS"), ",") {
		name, rule, ok := parseRule(part)
		if !ok {
			continue
		}
		if _, known := cfg.Rules[name]; !known {
			continue
		}
		cfg.Rules[name] = rule
	}
	return cfg
}

func parseRule(part string) (string, Rule, bool) {
	part = strings.TrimSpace(part)
	if part == "" {
		return "", Rule{}, false
	}
	fields := strings.Split(part, ":")
	if len(fields) != 3 {
		return "", Rule{}, false
	}
	burst, err := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)
	if err != nil || burst < 1 {
		return "", Rule{}, false
	}
	rate, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
	if err != nil || rate < 0 {
		return "", Rule{}, false
	}
	return strings.TrimSpace(fields[0]), Rule{Burst: burst, Rate: rate}, true
}

// Store is one token bucket. Take spends one token or reports the wait.
type Store interface {
	Take(ctx context.Context, key string, rule Rule, now time.Time) (allowed bool, wait time.Duration, err error)
}

// Request is one client RPC.
type Request struct {
	Method  string
	UserID  int64
	AuthKey int64
	IP      string
	Phone   string
}

// Decision is the limiter result. Allowed is false only in enforce mode.
// Limited is true when a bucket was empty, including log-only mode.
type Decision struct {
	Allowed bool
	Limited bool
	WaitSec int32
	Bucket  string
	Scope   string
}

// Limiter checks the buckets for one RPC.
type Limiter struct {
	store Store
	cfg   Config
	now   func() time.Time
}

func New(store Store, cfg Config) *Limiter {
	if cfg.Rules == nil {
		cfg.Rules = defaultRules()
	}
	if cfg.Mode == "" {
		cfg.Mode = ModeEnforce
	}
	return &Limiter{store: store, cfg: cfg, now: time.Now}
}

type charge struct {
	bucket string
	scope  string
	id     string
}

// Allow spends a token on each bucket that applies. Redis errors allow the
// call: a cache outage must not turn into a flood of false FLOOD_WAIT.
func (l *Limiter) Allow(ctx context.Context, in Request) Decision {
	ok := Decision{Allowed: true}
	if l == nil || l.store == nil || l.cfg.Mode == ModeOff {
		return ok
	}
	charges := chargesFor(in)
	if len(charges) == 0 {
		return ok
	}
	now := l.now()
	var worst time.Duration
	for _, ch := range charges {
		rule, known := l.cfg.Rules[ch.bucket]
		if !known || rule.Burst < 1 {
			continue
		}
		allowed, wait, err := l.store.Take(ctx, "flood:"+ch.bucket+":"+ch.scope+":"+ch.id, rule, now)
		if err != nil {
			logx.WithContext(ctx).Errorf("flood: redis %s: %v", ch.bucket, err)
			return ok
		}
		if allowed {
			continue
		}
		if wait > worst {
			worst = wait
			ok.Bucket = ch.bucket
			ok.Scope = ch.scope
		}
		ok.Limited = true
	}
	if !ok.Limited {
		return ok
	}
	sec := int32(worst / time.Second)
	if worst%time.Second != 0 {
		sec++
	}
	if sec < 1 {
		sec = 1
	}
	if sec > 3600 {
		sec = 3600
	}
	ok.WaitSec = sec
	if l.cfg.Mode == ModeLog {
		logx.WithContext(ctx).Infof("flood: log-only %s %s would wait %ds method %s", ok.Bucket, ok.Scope, ok.WaitSec, in.Method)
		ok.Allowed = true
		return ok
	}
	logx.WithContext(ctx).Infof("flood: FLOOD_WAIT_%d %s %s method %s", ok.WaitSec, ok.Bucket, ok.Scope, in.Method)
	ok.Allowed = false
	return ok
}

func chargesFor(in Request) []charge {
	class := classOf(in.Method)
	var out []charge
	if class != classUpload {
		if c, ok := subjectCharge(BucketGlobal, in); ok {
			out = append(out, c)
		}
	}
	switch class {
	case classSend:
		if c, ok := subjectCharge(BucketSend, in); ok {
			out = append(out, c)
		}
	case classSendCode:
		if in.Phone != "" {
			out = append(out, charge{BucketSendCodePhone, "phone", in.Phone})
		}
		if in.IP != "" {
			out = append(out, charge{BucketSendCodeIP, "ip", in.IP})
		}
	case classSignIn:
		if in.Phone != "" {
			out = append(out, charge{BucketSignInPhone, "phone", in.Phone})
		}
		if in.IP != "" {
			out = append(out, charge{BucketSignInIP, "ip", in.IP})
		}
	case classImport:
		if c, ok := subjectCharge(BucketImportContacts, in); ok {
			out = append(out, c)
		}
	case classResolve:
		if c, ok := subjectCharge(BucketResolve, in); ok {
			out = append(out, c)
		}
	case classSearch:
		if c, ok := subjectCharge(BucketContactsSearch, in); ok {
			out = append(out, c)
		}
	case classUpload:
		if c, ok := subjectCharge(BucketUpload, in); ok {
			out = append(out, c)
		}
	case classStatus:
		if c, ok := subjectCharge(BucketAccountStatus, in); ok {
			out = append(out, c)
		}
	case classAccount:
		if c, ok := subjectCharge(BucketAccountUpdate, in); ok {
			out = append(out, c)
		}
	case classReport:
		if c, ok := subjectCharge(BucketReport, in); ok {
			out = append(out, c)
		}
	case classWebPreview:
		if c, ok := subjectCharge(BucketWebPreview, in); ok {
			out = append(out, c)
		}
	}
	return out
}

func subjectCharge(bucket string, in Request) (charge, bool) {
	if in.UserID != 0 {
		return charge{bucket, "user", strconv.FormatInt(in.UserID, 10)}, true
	}
	if in.AuthKey != 0 {
		return charge{bucket, "auth", strconv.FormatInt(in.AuthKey, 10)}, true
	}
	return charge{}, false
}
