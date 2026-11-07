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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dao"
)

// AuthsessionResetAuthorization
// authsession.resetAuthorization user_id:long auth_key_id:long hash:long = Vector<long>;
func (c *AuthsessionCore) AuthsessionResetAuthorization(in *authsession.TLAuthsessionResetAuthorization) (*authsession.Vector_Long, error) {
	var (
		excludeKeyId = in.AuthKeyId
	)

	if excludeKeyId != 0 {
		myKeyData, err := c.svcCtx.Dao.QueryAuthKeyV2(c.ctx, in.AuthKeyId)
		if err != nil {
			c.Logger.Errorf("session.getAuthorizations - error: %v", err)
			return nil, err
		} else if myKeyData == nil {
			c.Logger.Errorf("session.getAuthorizations - error: %v", err)
			err = mtproto.ErrAuthKeyInvalid
			return nil, err
		} else if bound := myKeyData.BoundPermAuthKeyId(); bound != 0 {
			// A legacy perm row can leave PermAuthKeyId at 0. Treating that
			// as "exclude nobody" would delete the caller when hash is 0.
			excludeKeyId = bound
		}
	}

	// Account deletion passes auth_key_id 0 and must still clear every key.
	// Any real session is gated on its own date_created before a row changes.
	created := int64(0)
	if in.AuthKeyId != 0 {
		var found bool
		var err error
		created, found, err = c.svcCtx.Dao.AuthorizationCreated(c.ctx, excludeKeyId)
		if err != nil {
			c.Logger.Errorf("authsession.resetAuthorization - created: %v", err)
			return nil, err
		}
		if !found {
			c.Logger.Errorf("authsession.resetAuthorization - actor %d has no live auth_users row", excludeKeyId)
			return nil, mtproto.ErrAuthKeyUnregistered
		}
	}
	now := time.Now().Unix()
	// matched=true here only evaluates the 24h gate. A fresh session is
	// refused before ResetAuthorization, so a client cannot hide a device
	// that was not actually revoked.
	if err := dao.ResetAuthorizationError(in.AuthKeyId, created, now, in.Hash, true); err != nil {
		c.Logger.Errorf("authsession.resetAuthorization - refused: %v", err)
		return nil, err
	}

	keyIdList := c.svcCtx.Dao.ResetAuthorization(c.ctx, in.UserId, excludeKeyId, in.Hash)
	if err := dao.ResetAuthorizationError(in.AuthKeyId, created, now, in.Hash, len(keyIdList) > 0); err != nil {
		c.Logger.Errorf("authsession.resetAuthorization - hash: %v", err)
		return nil, err
	}

	// Notify the permanent key and any temp keys bound to it. The in-memory
	// session wrapper is keyed by the permanent id; a temp id is included so
	// a wrapper registered under that id is dropped too.
	notify := make([]int64, 0, len(keyIdList)*3)
	for _, keyId := range keyIdList {
		notify = append(notify, keyId)
		keyData, _ := c.svcCtx.Dao.QueryAuthKeyV2(c.ctx, keyId)
		if keyData == nil {
			continue
		}
		notify = append(notify, keyData.GetTempAuthKeyId(), keyData.GetMediaTempAuthKeyId())
	}

	return &authsession.Vector_Long{
		Datas: dao.UnionAuthKeyIDs(notify),
	}, nil
}
