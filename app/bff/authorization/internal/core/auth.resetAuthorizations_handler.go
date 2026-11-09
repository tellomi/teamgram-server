// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/pkg/push"
)

// AuthResetAuthorizations
// auth.resetAuthorizations#9fab0d1a = Bool;
//
// Terminates every authorization except the current one (auth_users rows
// soft-deleted, cache dropped). Each other key is then told to drop its
// in-memory session. A later RPC on those keys loads the deleted binding
// and fails as logged out (401 AUTH_KEY_UNREGISTERED).
//
// A session younger than 24 hours gets 406 FRESH_RESET_AUTHORISATION_FORBIDDEN
// from authsession and nothing is deleted. That error is returned here.
// Success is only returned after the other rows were actually revoked.
func (c *AuthorizationCore) AuthResetAuthorizations(in *mtproto.TLAuthResetAuthorizations) (*mtproto.Bool, error) {
	_ = in

	if c.MD == nil || c.MD.UserId == 0 || c.MD.PermAuthKeyId == 0 {
		c.Logger.Errorf("auth.resetAuthorizations - missing user or perm auth key")
		return nil, mtproto.ErrAuthKeyUnregistered
	}

	tKeyIdList, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionResetAuthorization(c.ctx, &authsession.TLAuthsessionResetAuthorization{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		Hash:      0,
	})
	if err != nil {
		c.Logger.Errorf("auth.resetAuthorizations - error: %v", err)
		return nil, err
	}

	revoked := push.RevokedAuthKeys([]int64{c.MD.PermAuthKeyId, c.MD.AuthId}, tKeyIdList.GetDatas())
	c.unregisterPushDevices(revoked...)

	for _, id := range revoked {
		// Same signals as account.resetAuthorization: refresh the other
		// client, then drop that auth key's session.
		_, _ = c.svcCtx.Dao.SyncClient.SyncUpdatesMe(c.ctx, &sync.TLSyncUpdatesMe{
			UserId:        c.MD.UserId,
			PermAuthKeyId: id,
			ServerId:      nil,
			AuthKeyId:     nil,
			SessionId:     nil,
			Updates:       mtproto.MakeTLUpdatesTooLong(nil).To_Updates(),
		})
		_, _ = c.svcCtx.Dao.SyncClient.SyncUpdatesMe(c.ctx, &sync.TLSyncUpdatesMe{
			UserId:        c.MD.UserId,
			PermAuthKeyId: id,
			ServerId:      nil,
			AuthKeyId:     nil,
			SessionId:     nil,
			Updates: mtproto.MakeTLUpdateAccountResetAuthorization(&mtproto.Updates{
				UserId:    c.MD.UserId,
				AuthKeyId: id,
			}).To_Updates(),
		})
	}

	return mtproto.BoolTrue, nil
}
