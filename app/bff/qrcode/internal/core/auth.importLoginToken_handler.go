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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"
)

// AuthImportLoginToken
// auth.importLoginToken#95ac5ce4 token:bytes = auth.LoginToken;
//
// https://core.telegram.org/method/auth.importLoginToken
// This server has one DC, so a token is not a cross-DC migrate payload.
// A blob that is not a live QR token is 400 AUTH_TOKEN_INVALID.
// A token this server already accepted is 400 AUTH_TOKEN_ALREADY_ACCEPTED.
// A token past its expiry is 400 AUTH_TOKEN_EXPIRED.
func (c *QrCodeCore) AuthImportLoginToken(in *mtproto.TLAuthImportLoginToken) (*mtproto.Auth_LoginToken, error) {
	token := []byte(nil)
	if in != nil {
		token = in.GetToken()
	}
	// acceptLoginToken uses 8 bytes of auth key id plus a 16-byte check.
	if len(token) != 24 {
		c.Logger.Errorf("auth.importLoginToken - invalid token length %d", len(token))
		return nil, mtproto.ErrAuthTokenInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	keyID := int64(binary.BigEndian.Uint64(token))
	qrCode, err := c.svcCtx.Dao.GetCacheQRLoginCode(c.ctx, keyID)
	if err != nil || qrCode == nil || !qrCode.CheckByToken(token) {
		c.Logger.Errorf("auth.importLoginToken - unknown token")
		return nil, mtproto.ErrAuthTokenInvalid
	}
	if model.QRExpired(qrCode.ExpireAt, time.Now().Unix()) {
		c.Logger.Errorf("auth.importLoginToken - expired")
		return nil, mtproto.ErrAuthTokenExpired
	}
	switch qrCode.State {
	case model.QRCodeStateAccepted, model.QRCodeStateSuccess:
		c.Logger.Errorf("auth.importLoginToken - already accepted")
		return nil, mtproto.ErrAuthTokenAlreadyAccepted
	default:
		// A same-DC QR token is finished by auth.exportLoginToken, not by
		// importing it as a migrate payload.
		c.Logger.Errorf("auth.importLoginToken - not a migrate token")
		return nil, mtproto.ErrAuthTokenInvalid
	}
}
