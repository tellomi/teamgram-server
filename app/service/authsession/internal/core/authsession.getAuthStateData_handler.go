/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"errors"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

// AuthsessionGetAuthStateData
// authsession.getAuthStateData auth_key_id:long = AuthKeyStateData;
func (c *AuthsessionCore) AuthsessionGetAuthStateData(in *authsession.TLAuthsessionGetAuthStateData) (*authsession.AuthKeyStateData, error) {
	var (
		inKeyId = in.GetAuthKeyId()
	)

	keyData, err := c.svcCtx.Dao.QueryAuthKeyV2(c.ctx, inKeyId)
	if err != nil {
		c.Logger.Errorf("queryAuthKeyV2(%d) is error: %v", inKeyId, err)
		return nil, err
	}
	permID := keyData.BoundPermAuthKeyId()
	if permID == 0 {
		c.Logger.Errorf("queryAuthKeyV2(%d) - PermAuthKeyId is empty", inKeyId)
		return nil, mtproto.ErrAuthKeyPermEmpty
	}

	cData, err := c.svcCtx.GetCacheAuthData(c.ctx, permID)
	if err != nil {
		if !errors.Is(err, sqlc.ErrNotFound) {
			c.Logger.Errorf("authsession.getAuthStateData - error: %v", err)
			return nil, err
		}
	}

	state := cData.ToAuthState()
	userID := cData.UserId()
	// A cloud-password login binds the key before auth.checkPassword. A
	// reconnect must not treat that auth_users row as a finished login.
	pending, pendingOK := c.svcCtx.Dao.PasswordLoginUser(c.ctx, permID)
	state, userID = passwordPendingState(state, userID, pending, pendingOK)

	return authsession.MakeTLAuthKeyStateData(&authsession.AuthKeyStateData{
		AuthKeyId:            inKeyId,
		KeyState:             int32(state),
		UserId:               userID,
		AccessHash:           0,
		Client:               cData.GetClient(),
		AndroidPushSessionId: mtproto.MakeFlagsInt64(cData.AndroidPushSessionId()),
	}).To_AuthKeyStateData(), nil
}

// passwordPendingState keeps a reconnecting key on the password step when
// auth_users already has a user and user_password_login still names that
// user. Without this, ToAuthState reports Normal and the client skips
// auth.checkPassword.
func passwordPendingState(state int, userID, pending int64, pendingOK bool) (int, int64) {
	if pendingOK && pending != 0 && (userID == 0 || userID == pending) {
		if userID == 0 {
			userID = pending
		}
		return mtproto.AuthStateNeedPassword, userID
	}
	return state, userID
}
