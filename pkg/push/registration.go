package push

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

// registrationUsers is the latest installation's account set. other_uids is
// Telegram's list of other accounts currently using this client; it is not
// an instruction to subscribe arbitrary users or to keep every historic owner.
func registrationUsers(d Device) []int64 {
	ids := []int64{d.UserID}
	for _, id := range d.OtherUIDs {
		if id <= 0 {
			continue
		}
		found := false
		for _, old := range ids {
			if old == id {
				found = true
				break
			}
		}
		if !found {
			ids = append(ids, id)
		}
	}
	return ids
}

func registrationOwns(d Device, uid int64) bool {
	for _, id := range registrationUsers(d) {
		if id == uid {
			return true
		}
	}
	return false
}

// Keep token transfer and the new row atomic. Serializable InnoDB range locks
// also serialize competing registrations of a token that has no row yet.
// Deadlocks/lock waits are retried as an entire transaction, never half a transfer.
func (m *MySQL) upsertRegistration(ctx context.Context, d Device, secret, other string, noMuted, sandbox int) error {
	for attempt := 0; ; attempt++ {
		err := m.upsertRegistrationTx(ctx, d, secret, other, noMuted, sandbox)
		var me *mysqlDriver.MySQLError
		if attempt >= 2 || !errors.As(err, &me) || (me.Number != 1213 && me.Number != 1205) {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *MySQL) upsertRegistrationTx(ctx context.Context, d Device, secret, other string, noMuted, sandbox int) error {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ids := registrationUsers(d)
	marks := make([]string, len(ids))
	args := []any{d.TokenType, d.Token, d.Token}
	for i, id := range ids {
		marks[i] = "?"
		args = append(args, id)
	}
	// The first comparison allows the token index to narrow the search. BINARY
	// prevents a case-insensitive collation from evicting a different FCM token.
	q := `DELETE FROM devices WHERE token_type = ? AND token = ? AND BINARY token = BINARY ? AND user_id NOT IN (` + strings.Join(marks, ",") + `)`
	if _, err = tx.ExecContext(ctx, q, args...); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM devices WHERE user_id = ? AND token_type = ? AND token = ? AND BINARY token = BINARY ? AND auth_key_id <> ?`, d.UserID, d.TokenType, d.Token, d.Token, d.AuthKeyID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO devices (auth_key_id, user_id, token_type, token, no_muted, locked_period, app_sandbox, secret, other_uids, lang_code, state)
VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?, 0)
ON DUPLICATE KEY UPDATE
  token = VALUES(token),
  no_muted = VALUES(no_muted),
  app_sandbox = VALUES(app_sandbox),
  secret = VALUES(secret),
  other_uids = VALUES(other_uids),
  lang_code = IF(VALUES(lang_code) = '', lang_code, VALUES(lang_code)),
  state = 0`, d.AuthKeyID, d.UserID, d.TokenType, d.Token, noMuted, sandbox, secret, other, d.LangCode); err != nil {
		return err
	}
	return tx.Commit()
}
