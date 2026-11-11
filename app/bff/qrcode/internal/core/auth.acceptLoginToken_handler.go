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
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/loginnotice"
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

	c.Logger.Infof("auth.acceptLoginToken - state %d", qrCode.State)

	if !qrCode.CheckByToken(in.Token) {
		err := mtproto.ErrAuthTokenInvalid
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}

	now := time.Now().Unix()
	if model.QRExpired(qrCode.ExpireAt, now) {
		err := mtproto.ErrAuthTokenExpired
		c.Logger.Errorf("auth.acceptLoginToken - expired")
		return nil, err
	}

	switch qrCode.State {
	case model.QRCodeStateNew:
	case model.QRCodeStateSuccess:
		// A finished login must not run again. A failed attempt stays Accepted
		// and can be resumed by the same user below.
		err := mtproto.ErrAuthTokenAccepted
		c.Logger.Errorf("auth.acceptLoginToken - finished")
		return nil, err
	case model.QRCodeStateAccepted:
		// The record already names who accepted it. Another caller must not
		// reach user lookup or bind this device to themselves.
		if qrCode.UserId != c.MD.UserId {
			err := mtproto.ErrAuthTokenAccepted
			c.Logger.Errorf("auth.acceptLoginToken - other user")
			return nil, err
		}
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

	remain := int(qrCode.ExpireAt - now)
	claimed, token, claimErr := c.svcCtx.Dao.ClaimQRLogin(c.ctx, keyId, c.MD.UserId, remain)
	if claimErr != nil {
		c.Logger.Errorf("auth.acceptLoginToken - claim: %v", claimErr)
		return nil, claimErr
	}
	switch decideQRAccept(qrCode.State, claimed, claimOwnerID(token), c.MD.UserId, qrCode.UserId) {
	case qrReject:
		err := mtproto.ErrAuthTokenAccepted
		c.Logger.Errorf("auth.acceptLoginToken - already claimed")
		return nil, err
	case qrMark:
		if err = c.svcCtx.Dao.MarkQRAccepted(c.ctx, keyId, c.MD.UserId, token); err != nil {
			c.Logger.Errorf("auth.acceptLoginToken - state")
			return nil, err
		}
	case qrResume:
		// The claim and the accepted state are already ours. Continue the
		// user lookup and bind; do not take the token away from this user.
	}

	user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}

	// A cloud password means this scan is not a finished login. The notice is
	// sent later by auth.checkPassword, for this new key, after the proof
	// matches. Sending it here would report a login that has not happened.
	passwordNeeded := false
	if has, hasErr := password.HasPassword(c.ctx, user.Id()); hasErr != nil {
		c.Logger.Errorf("auth.acceptLoginToken - password: %v", hasErr)
		return nil, hasErr
	} else if has || (c.svcCtx.Plugin != nil && c.svcCtx.Plugin.CheckSessionPasswordNeeded(c.ctx, user.Id())) {
		passwordNeeded = true
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

	// The device is the new key's initConnection model, not the phone that
	// scanned the code. A rejected or replayed token returned above.
	if qrLoginNotifies(qrCode.State, passwordNeeded) && c.svcCtx.Dao != nil {
		keyID := qrCode.PermAuthKeyId
		if keyID == 0 {
			keyID = qrCode.AuthKeyId
		}
		loginnotice.Push(c.svcCtx.Dao.MsgClient, user.Id(), keyID, loginnotice.Device(authorization.GetDeviceModel(), "", ""), time.Now())
	}

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

	_ = c.svcCtx.Dao.UpdateCacheQRLoginCode(c.ctx, keyId, map[string]string{
		"state": strconv.Itoa(model.QRCodeStateSuccess),
	})
	return authorization, nil
}

// qrLoginNotifies is true only when this accept finishes the login.
// A replayed or expired token is not state New. A cloud-password scan is
// not finished until auth.checkPassword.
func qrLoginNotifies(state int, passwordNeeded bool) bool {
	return state == model.QRCodeStateNew && !passwordNeeded
}

const (
	qrMark = iota
	qrResume
	qrReject
)

// decideQRAccept is the retry rule. A lost SETNX response leaves the state
// New while this user already owns the claim, so the retry must mark and
// continue. Accepted or Success for that same owner resumes the bind.
// Another user's claim is rejected.
func decideQRAccept(state int, won bool, owner, caller, recordedUser int64) int {
	if caller == 0 || owner != caller {
		return qrReject
	}
	switch state {
	case model.QRCodeStateSuccess:
		return qrReject
	case model.QRCodeStateAccepted:
		if recordedUser != caller {
			return qrReject
		}
		return qrResume
	case model.QRCodeStateNew:
		_ = won
		return qrMark
	default:
		return qrReject
	}
}

func claimOwnerID(token string) int64 {
	if token == "" {
		return 0
	}
	head := token
	if i := strings.IndexByte(token, ':'); i >= 0 {
		head = token[:i]
	}
	id, err := strconv.ParseInt(head, 10, 64)
	if err != nil {
		return 0
	}
	return id
}
