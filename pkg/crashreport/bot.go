package crashreport

import (
	"os"
	"strings"
	"unicode/utf8"
)

const (
	// EnvBot is the help.getAppConfig value for tellomi_crash_report_bot.
	// Empty means DefaultUsername. The account created by the migration is
	// always DefaultUsername; this only changes the string clients look up.
	EnvBot = "TELLOMI_CRASH_REPORT_BOT"
	// EnvToken is the admin token for the query listener. Empty leaves the
	// listener off. It is never written into the binary.
	EnvToken = "TELLOMI_CRASH_REPORT_TOKEN"
	// EnvAddr is the listen address. It must be loopback or a private IP.
	EnvAddr = "TELLOMI_CRASH_REPORT_ADDR"

	DefaultUsername = "TellomiBot"
	DisplayName     = "Tellomi 反馈"
	AccountID       = int64(777006)
	Class           = "crash-reports"
	defaultAddr     = "127.0.0.1:8818"
	// placeholderToken is stored on the bots row so the unique token index
	// has a value. auth.importBotAuthorization is not implemented, and this
	// string is not a credential.
	placeholderToken = "tellomi-crash-collector"
)

// Username is the name help.getAppConfig publishes. An invalid value falls
// back to DefaultUsername so a bad env does not advertise a name clients
// cannot resolve.
func Username() string {
	name := strings.TrimPrefix(strings.TrimSpace(os.Getenv(EnvBot)), "@")
	if name == "" || !validUsername(name) {
		return DefaultUsername
	}
	return name
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
