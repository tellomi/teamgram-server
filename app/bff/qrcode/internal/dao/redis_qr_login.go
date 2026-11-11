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

package dao

import (
	"context"
	"fmt"
	"strconv"

	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// qrCodeTimeout     int64 = 30 // salt timeout
	cacheQRCodePrefix = "qr_codes"
)

func genQRLoginCodeKey(authKeyId int64) string {
	return fmt.Sprintf("%s_%d", cacheQRCodePrefix, authKeyId)
}

func (d *Dao) GetCacheQRLoginCode(ctx context.Context, keyId int64) (code *model.QRCodeTransaction, err error) {
	var (
		key    = genQRLoginCodeKey(keyId)
		values map[string]string
	)

	values, err = d.kv.HgetallCtx(ctx, key)
	if err != nil {
		logx.WithContext(ctx).Errorf("conn.Do(HGETALL %s) error(%v)", key, err)
		return nil, err
	} else if len(values) == 0 {
		return
	}

	code = new(model.QRCodeTransaction)
	for k, v := range values {
		switch k {
		case "perm_auth_key_id":
			code.PermAuthKeyId, _ = strconv.ParseInt(v, 10, 64)
		case "session_id":
			code.SessionId, _ = strconv.ParseInt(v, 10, 64)
		case "auth_key_id":
			code.AuthKeyId, _ = strconv.ParseInt(v, 10, 64)
		case "server_id":
			code.ServerId = v
		case "api_id":
			v, _ := strconv.ParseInt(v, 10, 64)
			code.ApiId = int32(v)
		case "api_hash":
			code.ApiHash = v
		case "code_hash":
			code.CodeHash = v
		case "expire_at":
			code.ExpireAt, _ = strconv.ParseInt(v, 10, 64)
		case "user_id":
			code.UserId, _ = strconv.ParseInt(v, 10, 64)
		case "state":
			v, _ := strconv.ParseInt(v, 10, 64)
			code.State = int(v)
		}
	}

	return
}

func (d *Dao) PutCacheQRLoginCode(ctx context.Context, keyId int64, qrCode *model.QRCodeTransaction, expiredIn int) (err error) {
	var (
		key = genQRLoginCodeKey(keyId)

		args = map[string]string{
			"perm_auth_key_id": strconv.FormatInt(qrCode.PermAuthKeyId, 10),
			"session_id":       strconv.FormatInt(qrCode.SessionId, 10),
			"auth_key_id":      strconv.FormatInt(qrCode.AuthKeyId, 10),
			"server_id":        qrCode.ServerId,
			"api_id":           strconv.Itoa(int(qrCode.ApiId)),
			"api_hash":         qrCode.ApiHash,
			"code_hash":        qrCode.CodeHash,
			"expire_at":        strconv.FormatInt(qrCode.ExpireAt, 10),
			"state":            strconv.Itoa(qrCode.State),
			"user_id":          strconv.FormatInt(qrCode.UserId, 10),
		}
	)

	// TODO(@benqi): args error??
	if err = d.kv.HmsetCtx(ctx, key, args); err != nil {
		logx.WithContext(ctx).Error("conn.Send(HMSET %s,%v) error(%v)", key, args, err)
		return
	}

	if expiredIn > 0 {
		if _, err = d.kv.ExpireWithResultCtx(ctx, key, expiredIn+2); err != nil {
			logx.WithContext(ctx).Error("conn.Send(EXPIRE %d,%d) error(%v)", key, expiredIn, err)
			return
		}
	}

	return
}

// ClaimQRLogin is the single-use gate. SetnxEx is atomic across processes
// and DCs that share this Redis. The second caller gets false.
func (d *Dao) ClaimQRLogin(ctx context.Context, keyId, userID int64, seconds int) (bool, error) {
	if d == nil || d.kv == nil || keyId == 0 || userID == 0 {
		return false, fmt.Errorf("qr claim unavailable")
	}
	return claimOnce(ctx, d.kv.SetnxExCtx, fmt.Sprintf("qr_claim_%d", keyId), strconv.FormatInt(userID, 10), seconds)
}

func claimOnce(ctx context.Context, setnx func(context.Context, string, string, int) (bool, error), key, user string, seconds int) (bool, error) {
	if setnx == nil || key == "" || user == "" {
		return false, fmt.Errorf("qr claim unavailable")
	}
	if seconds < 1 {
		seconds = 1
	}
	return setnx(ctx, key, user, seconds)
}

// MarkQRAccepted records the scan. A failed write that left the token New
// releases this user's claim so a retry can take it. A write that landed
// keeps the claim and returns nil so the same attempt can finish the bind.
// A failed read keeps the claim, which blocks a second user.
func (d *Dao) MarkQRAccepted(ctx context.Context, keyId, userID int64) error {
	err := d.UpdateCacheQRLoginCode(ctx, keyId, map[string]string{
		"user_id": strconv.FormatInt(userID, 10),
		"state":   strconv.Itoa(model.QRCodeStateAccepted),
	})
	state := model.QRCodeStateNew
	var readErr error
	if err != nil {
		var again *model.QRCodeTransaction
		again, readErr = d.GetCacheQRLoginCode(ctx, keyId)
		if again != nil {
			state = again.State
		}
	}
	return qrAcceptAfterWrite(err, state, readErr, func() error {
		return d.ReleaseQRLogin(ctx, keyId, userID)
	})
}

// qrAcceptAfterWrite decides whether a failed state write still leaves the
// claim held. New or unreadable state releases only when the read succeeded
// and was still New. Any other state means the write landed.
func qrAcceptAfterWrite(writeErr error, state int, readErr error, release func() error) error {
	if writeErr == nil {
		return nil
	}
	if readErr != nil || state == model.QRCodeStateNew {
		if readErr == nil && release != nil {
			_ = release()
		}
		return writeErr
	}
	return nil
}

// ReleaseQRLogin deletes the claim only when it still names userID.
func (d *Dao) ReleaseQRLogin(ctx context.Context, keyId, userID int64) error {
	if d == nil || d.kv == nil || keyId == 0 || userID == 0 {
		return fmt.Errorf("qr claim unavailable")
	}
	key := fmt.Sprintf("qr_claim_%d", keyId)
	got, err := d.kv.GetCtx(ctx, key)
	if err != nil || got != strconv.FormatInt(userID, 10) {
		return err
	}
	_, err = d.kv.DelCtx(ctx, key)
	return err
}

// QRClaimOwner is the user id stored in the claim, or 0 when it is absent.
func (d *Dao) QRClaimOwner(ctx context.Context, keyId int64) (int64, error) {
	if d == nil || d.kv == nil || keyId == 0 {
		return 0, fmt.Errorf("qr claim unavailable")
	}
	got, err := d.kv.GetCtx(ctx, fmt.Sprintf("qr_claim_%d", keyId))
	if err != nil || got == "" {
		return 0, err
	}
	return strconv.ParseInt(got, 10, 64)
}

func (d *Dao) UpdateCacheQRLoginCode(ctx context.Context, keyId int64, values map[string]string) (err error) {
	var (
		key = genQRLoginCodeKey(keyId)
	)

	if err = d.kv.HmsetCtx(ctx, key, values); err != nil {
		logx.WithContext(ctx).Errorf("conn.HSET(%s) error(%v)", key, err)
	}

	return
}

func (d *Dao) DeleteCacheQRLoginCode(ctx context.Context, authKeyId int64) (err error) {
	key := genQRLoginCodeKey(authKeyId)

	if _, err = d.kv.DelCtx(ctx, key); err != nil {
		logx.WithContext(ctx).Errorf("conn.DEL(%s) error(%v)", key, err)
	}

	return
}
