package password

import (
	"context"
	"database/sql"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

// sensitiveFresh is the official 24 hour gate for privileged SRP calls
// such as channels.editCreator.
// https://core.telegram.org/method/channels.editCreator
const sensitiveFresh int64 = 24 * 60 * 60

// SessionCreated reports auth_users.date_created for a permanent auth key.
// Tests set it. Production leaves it nil and reads the same MySQL database
// the cloud-password store already opened.
// found is false when that live row does not exist. A zero created time is
// not treated as a brand-new session, matching account.resetAuthorization:
// a missing clock must not lock the owner out.
var SessionCreated func(ctx context.Context, authKeyID int64) (created int64, found bool, err error)

// CheckSensitive is the privileged password gate.
// No cloud password is PASSWORD_MISSING.
// A verifier stored less than 24 hours ago is PASSWORD_TOO_FRESH_<seconds>.
// A known session bound less than 24 hours ago is SESSION_TOO_FRESH_<seconds>.
// Anything else is the account SRP check: a wrong proof is PASSWORD_HASH_INVALID.
// https://core.telegram.org/method/channels.editCreator
func CheckSensitive(ctx context.Context, userID, authKeyID int64, proof *mtproto.InputCheckPasswordSRP) error {
	svc := Default()
	rec, err := svc.load(ctx, userID)
	if err != nil {
		return err
	}
	if !rec.HasPassword {
		return mtproto.ErrPasswordMissing
	}
	now := svc.now().Unix()
	if rec.PasswordSetAt > 0 && now >= rec.PasswordSetAt {
		elapsed := now - rec.PasswordSetAt
		if elapsed < sensitiveFresh {
			return mtproto.NewErrPasswordTooFreshX(int32(sensitiveFresh - elapsed))
		}
	}
	if authKeyID != 0 {
		created, found, err := sessionCreated(ctx, svc, authKeyID)
		if err != nil {
			return err
		}
		if found && created > 0 && now >= created {
			elapsed := now - created
			if elapsed < sensitiveFresh {
				return mtproto.NewErrSessionTooFreshX(int32(sensitiveFresh - elapsed))
			}
		}
	}
	return svc.verify(ctx, rec, authKeyID, proof)
}

func sessionCreated(ctx context.Context, svc *Service, authKeyID int64) (int64, bool, error) {
	if SessionCreated != nil {
		return SessionCreated(ctx, authKeyID)
	}
	if svc == nil {
		return 0, false, nil
	}
	ms, ok := svc.store.(*mysqlStore)
	if !ok || ms == nil || ms.db == nil {
		return 0, false, nil
	}
	created, found, err := ms.authUserCreated(ctx, authKeyID)
	if err != nil {
		// A missing auth_users table must not lock every owner out of a
		// transfer. The password proof is still required. A row that is
		// found and recent still returns SESSION_TOO_FRESH.
		logx.WithContext(ctx).Errorf("privileged session age: %v", err)
		return 0, false, nil
	}
	return created, found, nil
}

func (s *mysqlStore) authUserCreated(ctx context.Context, authKeyID int64) (int64, bool, error) {
	if s == nil || s.db == nil || authKeyID == 0 {
		return 0, false, nil
	}
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT date_created FROM auth_users WHERE auth_key_id = ? AND deleted = 0 LIMIT 1`, authKeyID).Scan(&created)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return created, true, nil
}
