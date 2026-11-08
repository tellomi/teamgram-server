package enforcement

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"

	"github.com/zeromicro/go-zero/core/logx"
)

var (
	gateMu  sync.RWMutex
	current *Service
)

// SetCurrent installs the process gate. Nil clears it. Tests must restore it.
func SetCurrent(s *Service) {
	gateMu.Lock()
	current = s
	gateMu.Unlock()
}

// Current is the installed service. Nil means enforcement is not configured.
func Current() *Service {
	gateMu.RLock()
	defer gateMu.RUnlock()
	return current
}

// InstallFromEnv opens TELLOMI_ENFORCEMENT_MYSQL when it names a database other
// than the chat schema. Memory is never installed from the environment.
// TELLOMI_ENFORCEMENT_CREATE=1 allows new cases. An empty DSN installs nothing.
// When TELLOMI_AI=1 and TELLOMI_AI_API_KEY are set, appeals use pkg/tellomiai.Client.
func InstallFromEnv() *Service {
	dsn := strings.TrimSpace(os.Getenv("TELLOMI_ENFORCEMENT_MYSQL"))
	if dsn == "" || os.Getenv("TELLOMI_ENFORCEMENT_STORE") == "memory" {
		return nil
	}
	cfg := Config{CreateEnabled: os.Getenv("TELLOMI_ENFORCEMENT_CREATE") == "1"}
	if os.Getenv(tellomiai.EnvEnabled) == "1" && strings.TrimSpace(os.Getenv(tellomiai.EnvAPIKey)) != "" {
		timeout := 12 * time.Second
		cfg.Judge = ModelJudge{
			Model: os.Getenv(tellomiai.EnvModelPrimary),
			Completer: tellomiai.NewClient(
				os.Getenv(tellomiai.EnvBaseURL),
				os.Getenv(tellomiai.EnvAPIKey),
				os.Getenv(tellomiai.EnvModelPrimary),
				os.Getenv(tellomiai.EnvModelSmall),
				os.Getenv(tellomiai.EnvModelFallback),
				timeout,
			),
		}
	}
	s, err := OpenMySQL(dsn, cfg)
	if err != nil {
		logx.Errorf("enforcement: separate database was not installed")
		return nil
	}
	SetCurrent(s)
	return s
}

// RPC maps a decision code onto an MTProto error. An empty code is success.
func RPC(code string) error {
	switch code {
	case "":
		return nil
	case "IDENTIFIER_RESTRICTED":
		return mtproto.ErrIdentifierRestricted
	case "ACCOUNT_READ_ONLY":
		return mtproto.ErrAccountReadOnly
	case "ACCOUNT_SCOPED_RESTRICTION":
		return mtproto.ErrAccountScopedRestriction
	case "ACCOUNT_PERMANENT_DISABLED":
		return mtproto.ErrAccountPermanentDisabled
	case "ACCOUNT_SECURITY_LOCK":
		return mtproto.ErrAccountSecurityLock
	default:
		return mtproto.ErrAccountScopedRestriction
	}
}

// LoginStage is the handler entry. A nil service allows an ordinary session.
func LoginStage(userID int64, identifier, kind string, passwordSatisfied bool, now time.Time) Login {
	s := Current()
	if s == nil {
		return Login{BindOrdinary: true}
	}
	return s.Stage(userID, identifier, kind, passwordSatisfied, now)
}

// SignupStage refuses a fresh account when the identifier is already bound.
func SignupStage(identifier, kind string, now time.Time) (Login, bool) {
	s := Current()
	if s == nil {
		return Login{}, false
	}
	return s.SignupInstead(identifier, kind, now)
}

// Reject runs a protected action. A nil service allows it.
func Reject(userID int64, action string, stranger bool, now time.Time) error {
	s := Current()
	if s == nil || userID == 0 {
		return nil
	}
	d := s.Decide(userID, action, stranger, now)
	if d.Allowed {
		return nil
	}
	return RPC(d.Code)
}

// ContactMatters reports that a stranger send is refused while a contact send is not.
func ContactMatters(userID int64, now time.Time) bool {
	s := Current()
	if s == nil || userID == 0 {
		return false
	}
	stranger := s.Decide(userID, "send_message", true, now)
	contact := s.Decide(userID, "send_message", false, now)
	return !stranger.Allowed && contact.Allowed
}

// CheckCredential is the content-engine recheck for a credential that was
// already issued. Permanent disable refuses both read and write. A read-only
// freeze refuses write and allows read. A discover-post scope refuses write only.
func CheckCredential(userID int64, write bool, now time.Time) Decision {
	s := Current()
	if s == nil || userID == 0 {
		return Decision{Allowed: true}
	}
	action := "discover_read"
	if write {
		action = "discover_post"
	}
	return s.Decide(userID, action, false, now)
}

// OrdinarySessionBlocked reports a permanent disable for an already-bound session.
func OrdinarySessionBlocked(userID int64, now time.Time) bool {
	s := Current()
	if s == nil || userID == 0 {
		return false
	}
	d := s.Decide(userID, "ordinary_session", false, now)
	return !d.Allowed && d.Code == "ACCOUNT_PERMANENT_DISABLED"
}
