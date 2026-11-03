// Package deletion is the shared account.deleteAccount decision.
//
// https://core.telegram.org/api/account-deletion
// No cloud password: delete now.
// Cloud password plus a correct proof: delete now.
// Cloud password and no proof: if that verifier is older than seven days
// (or the age is unknown) and the account was active in the last seven days,
// wait seven days. Otherwise delete now.
package deletion

import "time"

const Wait = 7 * 24 * time.Hour

type Action int

const (
	DeleteNow Action = iota
	Delay
)

type Input struct {
	Now               time.Time
	HasPassword       bool
	PasswordSetAt     time.Time // zero: age unknown, treated as older than Wait
	ActiveWithin7Days bool
	PasswordProvided  bool
	PasswordAccepted  bool
}

type Result struct {
	Action      Action
	WaitSeconds int32
}

// Decide applies the Telegram deletion rule. A wrong proof is not an action:
// the caller returns PASSWORD_HASH_INVALID before Decide.
func Decide(in Input) Result {
	// A rejected proof never reaches Decide. The RPC returns
	// PASSWORD_HASH_INVALID first.
	if !in.HasPassword || in.PasswordAccepted {
		return Result{Action: DeleteNow}
	}
	old := in.PasswordSetAt.IsZero() || in.Now.Sub(in.PasswordSetAt) > Wait
	if old && in.ActiveWithin7Days {
		sec := int32(Wait / time.Second)
		if sec < 1 {
			sec = 1
		}
		return Result{Action: Delay, WaitSeconds: sec}
	}
	return Result{Action: DeleteNow}
}
