package dao

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dao/mysql_dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

// AuthorizationAutoconfirmPeriod is how long a new session stays
// authorization.unconfirmed before account.getAuthorizations stops reporting
// the flag. Official clients use the same 7 days when
// help.getAppConfig omits authorization_autoconfirm_period, and this server
// publishes that value.
const AuthorizationAutoconfirmPeriod int64 = 7 * 24 * 60 * 60

const authorizationSettingsMigration = "migrate-20261103-authorization-settings.sql"

// authorizationSettingsPatch is one account.changeAuthorizationSettings call.
// A bool flag that was absent on the wire has the matching Set field false.
type authorizationSettingsPatch struct {
	Confirmed                 bool
	SetEncrypted              bool
	EncryptedRequestsDisabled bool
	SetCall                   bool
	CallRequestsDisabled      bool
}

func authorizationStillUnconfirmed(stored bool, dateCreated, now int64) bool {
	if !stored {
		return false
	}
	if dateCreated > 0 && AuthorizationAutoconfirmPeriod > 0 && now >= dateCreated+AuthorizationAutoconfirmPeriod {
		return false
	}
	return true
}

// newSessionStartsUnconfirmed reports whether a just-inserted auth_users row
// should be authorization.unconfirmed. liveSessions includes the new row.
// The first session has nothing to confirm it from.
func newSessionStartsUnconfirmed(liveSessions int) bool {
	return liveSessions > 1
}

func applyAuthorizationSettings(row dataobject.AuthUsersDO, patch authorizationSettingsPatch) dataobject.AuthUsersDO {
	if patch.SetEncrypted {
		row.EncryptedRequestsDisabled = patch.EncryptedRequestsDisabled
	}
	if patch.SetCall {
		row.CallRequestsDisabled = patch.CallRequestsDisabled
	}
	if patch.Confirmed {
		row.Unconfirmed = false
	}
	return row
}

func authorizationSettingsChanged(before, after dataobject.AuthUsersDO) bool {
	return before.EncryptedRequestsDisabled != after.EncryptedRequestsDisabled ||
		before.CallRequestsDisabled != after.CallRequestsDisabled ||
		before.Unconfirmed != after.Unconfirmed
}

// pickAuthorizationSettings finds the auth_users row for a hash from
// account.getAuthorizations. hash 0 is the caller's own session: getAuthorizations
// zeroes that hash, and the devices screen sends 0 back. A stored hash of 0
// is not a match for that case.
func pickAuthorizationSettings(rows []dataobject.AuthUsersDO, currentAuthKeyID, hash int64) (dataobject.AuthUsersDO, bool) {
	if hash == 0 {
		if currentAuthKeyID == 0 {
			return dataobject.AuthUsersDO{}, false
		}
		for _, row := range rows {
			if row.AuthKeyId == currentAuthKeyID {
				return row, true
			}
		}
		return dataobject.AuthUsersDO{}, false
	}
	var found dataobject.AuthUsersDO
	n := 0
	for _, row := range rows {
		if row.Hash == hash {
			found = row
			n++
		}
	}
	if n != 1 {
		return dataobject.AuthUsersDO{}, false
	}
	return found, true
}

func applyStoredAuthorizationFlags(auth *mtproto.Authorization, row dataobject.AuthUsersDO, now int64) {
	if auth == nil {
		return
	}
	auth.EncryptedRequestsDisabled = row.EncryptedRequestsDisabled
	auth.CallRequestsDisabled = row.CallRequestsDisabled
	auth.Unconfirmed = authorizationStillUnconfirmed(row.Unconfirmed, row.DateCreated, now)
}

func mysqlUnknownColumn(err error) bool {
	if err == nil {
		return false
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1054 {
		return true
	}
	return strings.Contains(err.Error(), "Unknown column")
}

func (d *Dao) authUsersDAO() *mysql_dao.AuthUsersDAO {
	if d == nil || d.Mysql == nil {
		return nil
	}
	return d.AuthUsersDAO
}

func (d *Dao) loadAuthorizationSettings(ctx context.Context, userID int64) ([]dataobject.AuthUsersDO, error) {
	dao := d.authUsersDAO()
	if dao == nil {
		return nil, errors.New("auth users dao is nil")
	}
	return dao.SelectAuthorizationSettingsByUserId(ctx, userID)
}

// ChangeAuthorizationSettings writes the flags account.getAuthorizations reads back.
// hash 0 updates currentAuthKeyID, the caller's permanent auth key. Any other
// hash must match exactly one live auth_users.hash for this user.
func (d *Dao) ChangeAuthorizationSettings(ctx context.Context, currentAuthKeyID int64, in *authsession.TLAuthsessionChangeAuthorizationSettings) error {
	if in == nil {
		return mtproto.ErrHashInvalid
	}
	userID := in.GetUserId()
	hash := in.GetHash()
	patch := settingsPatchFromRequest(in)
	rows, err := d.loadAuthorizationSettings(ctx, userID)
	if err != nil {
		if mysqlUnknownColumn(err) {
			logx.WithContext(ctx).Errorf("auth_users settings columns missing; apply teamgramd/deploy/sql/%s", authorizationSettingsMigration)
		} else {
			logx.WithContext(ctx).Errorf("changeAuthorizationSettings user_id=%d: %v", userID, err)
		}
		return mtproto.ErrInternalServerError
	}
	row, ok := pickAuthorizationSettings(rows, currentAuthKeyID, hash)
	if !ok {
		return mtproto.ErrHashInvalid
	}
	next := applyAuthorizationSettings(row, patch)
	if !authorizationSettingsChanged(row, next) {
		return nil
	}
	affected, err := d.AuthUsersDAO.UpdateAuthorizationSettings(ctx, next.EncryptedRequestsDisabled, next.CallRequestsDisabled, next.Unconfirmed, row.Id)
	if err != nil {
		if mysqlUnknownColumn(err) {
			logx.WithContext(ctx).Errorf("auth_users settings columns missing; apply teamgramd/deploy/sql/%s", authorizationSettingsMigration)
		} else {
			logx.WithContext(ctx).Errorf("changeAuthorizationSettings update id=%d: %v", row.Id, err)
		}
		return mtproto.ErrInternalServerError
	}
	if affected == 0 {
		return mtproto.ErrHashInvalid
	}
	return nil
}

// authorizationSettingsForUser is the flag map GetAuthorizations copies onto
// each authorization. A database that has not applied the migration yet
// returns an empty map so the device list still renders.
func (d *Dao) authorizationSettingsForUser(ctx context.Context, userID int64, now int64) map[int64]dataobject.AuthUsersDO {
	rows, err := d.loadAuthorizationSettings(ctx, userID)
	if err != nil {
		if mysqlUnknownColumn(err) {
			logx.WithContext(ctx).Errorf("auth_users settings columns missing; apply teamgramd/deploy/sql/%s", authorizationSettingsMigration)
		} else {
			logx.WithContext(ctx).Errorf("authorization settings user_id=%d: %v", userID, err)
		}
		return nil
	}
	out := make(map[int64]dataobject.AuthUsersDO, len(rows))
	for _, row := range rows {
		row.Unconfirmed = authorizationStillUnconfirmed(row.Unconfirmed, row.DateCreated, now)
		out[row.AuthKeyId] = row
	}
	return out
}

func (d *Dao) applyAuthorizationKeyFlags(ctx context.Context, authKeyID int64, auth *mtproto.Authorization) {
	dao := d.authUsersDAO()
	if dao == nil || auth == nil || authKeyID == 0 {
		return
	}
	row, err := dao.SelectAuthorizationSettingsByAuthKeyId(ctx, authKeyID)
	if err != nil {
		if mysqlUnknownColumn(err) {
			logx.WithContext(ctx).Errorf("auth_users settings columns missing; apply teamgramd/deploy/sql/%s", authorizationSettingsMigration)
		} else {
			logx.WithContext(ctx).Errorf("authorization settings auth_key_id=%d: %v", authKeyID, err)
		}
		return
	}
	if row == nil {
		return
	}
	applyStoredAuthorizationFlags(auth, *row, time.Now().Unix())
}

// markNewAuthorizationUnconfirmed sets unconfirmed on a row that was just
// inserted when the user already had another live session. Login keeps going
// when the column is not there yet.
func (d *Dao) markNewAuthorizationUnconfirmed(ctx context.Context, userID, authKeyID int64) {
	dao := d.authUsersDAO()
	if dao == nil {
		return
	}
	n, err := dao.CountLiveByUserId(ctx, userID)
	if err != nil {
		logx.WithContext(ctx).Errorf("count auth_users user_id=%d: %v", userID, err)
		return
	}
	if !newSessionStartsUnconfirmed(n) {
		return
	}
	if _, err = dao.UpdateUnconfirmed(ctx, authKeyID, userID); err != nil {
		if mysqlUnknownColumn(err) {
			logx.WithContext(ctx).Errorf("auth_users settings columns missing; apply teamgramd/deploy/sql/%s", authorizationSettingsMigration)
			return
		}
		logx.WithContext(ctx).Errorf("unconfirmed auth_key_id=%d user_id=%d: %v", authKeyID, userID, err)
	}
}

// settingsPatchFromRequest maps the internal RPC onto the stored flags.
func settingsPatchFromRequest(in *authsession.TLAuthsessionChangeAuthorizationSettings) authorizationSettingsPatch {
	if in == nil {
		return authorizationSettingsPatch{}
	}
	return authorizationSettingsPatch{
		Confirmed:                 in.GetConfirmed(),
		SetEncrypted:              in.GetSetEncryptedRequestsDisabled(),
		EncryptedRequestsDisabled: in.GetEncryptedRequestsDisabled(),
		SetCall:                   in.GetSetCallRequestsDisabled(),
		CallRequestsDisabled:      in.GetCallRequestsDisabled(),
	}
}
