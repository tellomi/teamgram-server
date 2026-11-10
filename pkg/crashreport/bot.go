package crashreport

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// EnvBot is the help.getAppConfig value for tellomi_crash_report_bot.
	// Empty means DefaultUsername. The account created by the migration is
	// always DefaultUsername; this only changes the string clients look up.
	EnvBot = "TELLOMI_CRASH_REPORT_BOT"
	// EnvToken is the admin token for the query listener. Empty leaves the
	// listener off. It is never written into the binary.
	EnvToken = "TELLOMI_CRASH_REPORT_TOKEN"
	// EnvAddr is the listen address. Only loopback (127.0.0.1, ::1, localhost) is accepted.
	EnvAddr = "TELLOMI_CRASH_REPORT_ADDR"
	// EnvMaxBytes caps a crash-report object. Larger files are recorded and not copied.
	EnvMaxBytes = "TELLOMI_CRASH_REPORT_MAX_BYTES"

	DefaultUsername = "TellomiBot"
	DisplayName     = "Tellomi 反馈"
	AccountID       = int64(777006)
	Class           = "crash-reports"
	defaultAddr     = "127.0.0.1:8819"
	// DefaultMaxBytes is 20 MiB.
	DefaultMaxBytes int64 = 20 << 20
	StatusStored          = "stored"
	StatusSkipped         = "skipped"
	// placeholderToken is stored on the bots row so the unique token index
	// has a value. auth.importBotAuthorization is not implemented, and this
	// string is not a credential.
	placeholderToken = "tellomi-crash-collector"
)

// published is the username stored for user 777006. help.getAppConfig uses it.
// TELLOMI_CRASH_REPORT_BOT does not replace it. A mismatch is logged at startup.
var published atomic.Value

// Username is the name help.getAppConfig publishes. Until startup reads the
// bot row, that name is DefaultUsername.
func Username() string {
	if v, ok := published.Load().(string); ok && v != "" {
		return v
	}
	return DefaultUsername
}

// UseDBUsername publishes the username stored for user 777006.
// TELLOMI_CRASH_REPORT_BOT is only a hint: when it differs, startup logs a
// warning and clients still see the database name. mismatch reports that case.
func UseDBUsername(dbName string) (mismatch bool) {
	dbName = strings.TrimSpace(dbName)
	if dbName == "" {
		dbName = DefaultUsername
	}
	cfg := strings.TrimPrefix(strings.TrimSpace(os.Getenv(EnvBot)), "@")
	if cfg != "" && !strings.EqualFold(cfg, dbName) {
		mismatch = true
		logx.Errorf("crash report: %s=%s does not match user %d username %s; publishing %s", EnvBot, cfg, AccountID, dbName, dbName)
	}
	published.Store(dbName)
	return mismatch
}

// MaxBytes is the largest object copied into crash-reports/. Unset or invalid
// values use DefaultMaxBytes. Zero means every non-empty body is skipped.
func MaxBytes() int64 {
	raw := strings.TrimSpace(os.Getenv(EnvMaxBytes))
	if raw == "" {
		return DefaultMaxBytes
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return DefaultMaxBytes
	}
	return n
}

func validUsername(name string) bool {
	n := utf8.RuneCountInString(name)
	if n < 4 || n > 32 || strings.HasPrefix(name, "_") || strings.HasSuffix(name, "_") {
		return false
	}
	for i, r := range name {
		if i == 0 && !isLetter(r) {
			return false
		}
		if !isLetter(r) && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
