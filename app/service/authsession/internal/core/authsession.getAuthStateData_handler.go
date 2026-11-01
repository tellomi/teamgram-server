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
	// A cloud-password login binds the key and records the user here so the
	// session stays on the password step across a process restart.
	if pending, ok := c.svcCtx.Dao.PasswordLoginUser(c.ctx, permID); ok && (userID == 0 || userID == pending) {
		state = mtproto.AuthStateNeedPassword
		if userID == 0 {
			userID = pending
		}
	}

	return authsession.MakeTLAuthKeyStateData(&authsession.AuthKeyStateData{
		AuthKeyId:            inKeyId,
		KeyState:             int32(state),
		UserId:               userID,
		AccessHash:           0,
		Client:               cData.GetClient(),
		AndroidPushSessionId: mtproto.MakeFlagsInt64(cData.AndroidPushSessionId()),
	}).To_AuthKeyStateData(), nil
}
