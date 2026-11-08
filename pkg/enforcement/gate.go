package enforcement

import (
	"os"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
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

// InstallFromEnv enables an in-memory store only when TELLOMI_ENFORCEMENT_STORE=memory.
// Creation stays off unless TELLOMI_ENFORCEMENT_CREATE=1. An empty environment
// installs nothing and does not change login.
func InstallFromEnv() *Service {
	if os.Getenv("TELLOMI_ENFORCEMENT_STORE") != "memory" {
		return nil
	}
	s := New(Config{CreateEnabled: os.Getenv("TELLOMI_ENFORCEMENT_CREATE") == "1"})
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

// OrdinarySessionBlocked reports a permanent disable for an already-bound session.
func OrdinarySessionBlocked(userID int64, now time.Time) bool {
	s := Current()
	if s == nil || userID == 0 {
		return false
	}
	d := s.Decide(userID, "ordinary_session", false, now)
	return !d.Allowed && d.Code == "ACCOUNT_PERMANENT_DISABLED"
}
