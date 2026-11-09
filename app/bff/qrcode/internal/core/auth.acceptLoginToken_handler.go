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
	"encoding/binary"
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/enforcement"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// AuthAcceptLoginToken
// auth.acceptLoginToken#e894ad4d token:bytes = Authorization;
func (c *QrCodeCore) AuthAcceptLoginToken(in *mtproto.TLAuthAcceptLoginToken) (*mtproto.Authorization, error) {
	// 8 + 16
	if len(in.Token) != 24 {
		err := mtproto.ErrAuthTokenInvalid
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}

	var (
		keyId = int64(binary.BigEndian.Uint64(in.Token))
	)

	qrCode, err := c.svcCtx.Dao.GetCacheQRLoginCode(c.ctx, keyId)
	if err != nil || qrCode == nil {
		err := mtproto.ErrAuthTokenExpired
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}

	c.Logger.Infof("auth.acceptLoginToken - qrCode: %#v", qrCode)

	if !qrCode.CheckByToken(in.Token) {
		err := mtproto.ErrAuthTokenInvalid
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}

	if qrCode.ExpireAt >= time.Now().Unix() {
		//s.AuthCore.DeleteQRCode(ctx, keyId)
		//err := mtproto.ErrAuthTokenExpired
		//log.Errorf("auth.acceptLoginToken - error: %v", err)
		//return nil, err
	}

	switch qrCode.State {
	case model.QRCodeStateNew:
		// ok
	case model.QRCodeStateAccepted, model.QRCodeStateSuccess:
		err := mtproto.ErrAuthTokenAccepted
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	default:
		err := mtproto.ErrAuthTokenInvalid
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}

	if stage := enforcement.LoginStage(c.MD.UserId, "", "", true, time.Now()); !stage.BindOrdinary {
		if err = enforcement.RPC(stage.Code); err != nil {
			c.Logger.Errorf("auth.acceptLoginToken - sanction")
			return nil, err
		}
	}

	c.svcCtx.Dao.UpdateCacheQRLoginCode(c.ctx, keyId, map[string]string{
		"user_id": strconv.FormatInt(c.MD.UserId, 10),
		"state":   strconv.Itoa(model.QRCodeStateAccepted),
	})

	user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}

	// Write the password step before the new device's auth key is bound.
	// exportLoginToken then returns SESSION_PASSWORD_NEEDED instead of a login.
	if has, hasErr := password.HasPassword(c.ctx, user.Id()); hasErr != nil {
		c.Logger.Errorf("auth.acceptLoginToken - password: %v", hasErr)
		return nil, hasErr
	} else if has || (c.svcCtx.Plugin != nil && c.svcCtx.Plugin.CheckSessionPasswordNeeded(c.ctx, user.Id())) {
		if err = password.NoteLogin(c.ctx, qrCode.PermAuthKeyId, user.Id()); err != nil {
			c.Logger.Errorf("auth.acceptLoginToken - password: %v", err)
			return nil, err
		}
	}

	// Bind authKeyId and userId
	hash, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionBindAuthKeyUser(c.ctx, &authsession.TLAuthsessionBindAuthKeyUser{
		AuthKeyId: qrCode.AuthKeyId,
		UserId:    user.Id(),
	})
	if err != nil {
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}
	authorization, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionGetAuthorization(c.ctx, &authsession.TLAuthsessionGetAuthorization{
		AuthKeyId: qrCode.AuthKeyId,
	})
	if err != nil {
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}

	authorization.DateCreated = int32(time.Now().Unix())
	authorization.DateActive = authorization.DateCreated
	authorization.Hash = hash.V

	c.svcCtx.Dao.SyncClient.SyncUpdatesMe(
		c.ctx,
		&sync.TLSyncUpdatesMe{
			UserId:        user.Id(),
			PermAuthKeyId: qrCode.PermAuthKeyId,
			ServerId:      &wrapperspb.StringValue{Value: qrCode.ServerId},
			AuthKeyId:     &wrapperspb.Int64Value{Value: qrCode.AuthKeyId},
			SessionId:     &wrapperspb.Int64Value{Value: qrCode.SessionId},
			Updates: mtproto.MakeTLUpdateShort(&mtproto.Updates{
				Update: mtproto.MakeTLUpdateLoginToken(nil).To_Update(),
				Date:   int32(time.Now().Unix()),
			}).To_Updates(),
		})

	return authorization, nil
}
