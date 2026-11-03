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
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

// authorizationSettingsFields is the subset of account.changeAuthorizationSettings
// that changes stored state. A Bool that the client left unset stays unset.
type authorizationSettingsFields struct {
	Confirmed                 bool
	SetEncrypted              bool
	EncryptedRequestsDisabled bool
	SetCall                   bool
	CallRequestsDisabled      bool
}

func changeAuthorizationSettingsFields(in *mtproto.TLAccountChangeAuthorizationSettings) authorizationSettingsFields {
	if in == nil {
		return authorizationSettingsFields{}
	}
	out := authorizationSettingsFields{Confirmed: in.GetConfirmed()}
	if b := in.GetEncryptedRequestsDisabled(); b != nil {
		out.SetEncrypted = true
		out.EncryptedRequestsDisabled = mtproto.FromBool(b)
	}
	if b := in.GetCallRequestsDisabled(); b != nil {
		out.SetCall = true
		out.CallRequestsDisabled = mtproto.FromBool(b)
	}
	return out
}

// AccountChangeAuthorizationSettings
// account.changeAuthorizationSettings#40f48462 flags:# confirmed:flags.3?true hash:long encrypted_requests_disabled:flags.0?Bool call_requests_disabled:flags.1?Bool = Bool;
func (c *AuthorizationCore) AccountChangeAuthorizationSettings(in *mtproto.TLAccountChangeAuthorizationSettings) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 || c.MD.PermAuthKeyId == 0 {
		c.Logger.Errorf("account.changeAuthorizationSettings - missing user or perm auth key")
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrHashInvalid
	}
	fields := changeAuthorizationSettingsFields(in)
	res, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionChangeAuthorizationSettings(c.ctx, &authsession.TLAuthsessionChangeAuthorizationSettings{
		UserId:                       c.MD.UserId,
		AuthKeyId:                    c.MD.PermAuthKeyId,
		Hash:                         in.GetHash(),
		Confirmed:                    fields.Confirmed,
		SetEncryptedRequestsDisabled: fields.SetEncrypted,
		EncryptedRequestsDisabled:    fields.EncryptedRequestsDisabled,
		SetCallRequestsDisabled:      fields.SetCall,
		CallRequestsDisabled:         fields.CallRequestsDisabled,
	})
	if err != nil {
		c.Logger.Errorf("account.changeAuthorizationSettings - error: %v", err)
		return nil, err
	}
	if res == nil {
		return mtproto.BoolTrue, nil
	}
	return res, nil
}
