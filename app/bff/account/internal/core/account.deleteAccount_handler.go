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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/account/deletion"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AccountDeleteAccount
// account.deleteAccount#a2c0cf74 flags:# reason:string password:flags.0?InputCheckPasswordSRP = Bool;
// https://core.telegram.org/api/account-deletion
func (c *AccountCore) AccountDeleteAccount(in *mtproto.TLAccountDeleteAccount) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	me, err := c.svcCtx.Dao.UserClient.UserGetUserDataById(c.ctx, &user.TLUserGetUserDataById{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("account.deleteAccount - user: %v", err)
		return nil, err
	}
	has, setAt, _, err := password.Info(c.ctx, c.MD.UserId)
	if err != nil {
		c.Logger.Errorf("account.deleteAccount - password: %v", err)
		return nil, err
	}
	provided := srpProvided(in.GetPassword())
	accepted := false
	if has && provided {
		if err = password.Verify(c.ctx, c.MD.UserId, c.permAuthKey(), in.GetPassword()); err != nil {
			c.Logger.Errorf("account.deleteAccount - password check failed")
			return nil, err
		}
		accepted = true
	}
	var set time.Time
	if setAt > 0 {
		set = time.Unix(setAt, 0)
	}
	decision := deletion.Decide(deletion.Input{
		Now:               time.Now(),
		HasPassword:       has,
		PasswordSetAt:     set,
		ActiveWithin7Days: true,
		PasswordProvided:  provided,
		PasswordAccepted:  accepted,
	})
	reason := trimReason(in.GetReason())
	if decision.Action == deletion.Delay {
		when := time.Now().Add(deletion.Wait).Unix()
		if err = deletion.Schedule(c.ctx, c.MD.UserId, when, reason); err != nil {
			c.Logger.Errorf("account.deleteAccount - schedule: %v", err)
			return nil, err
		}
		c.notifyDeletionScheduled()
		c.Logger.Infof("account.deleteAccount - scheduled user=%d", c.MD.UserId)
		return nil, mtproto.NewErr2faConfirmWaitX(decision.WaitSeconds)
	}
	_ = deletion.Cancel(c.ctx, c.MD.UserId)
	return c.purgeAccount(me, reason)
}

// PurgeAccount runs the same cleanup as a completed account.deleteAccount.
// The web form calls it after the SMS code and, when set, the cloud password.
func (c *AccountCore) PurgeAccount(userID int64, reason string) error {
	if c.MD == nil {
		c.MD = &metadata.RpcMetadata{UserId: userID}
	} else {
		c.MD.UserId = userID
	}
	me, err := c.svcCtx.Dao.UserClient.UserGetUserDataById(c.ctx, &user.TLUserGetUserDataById{
		UserId: userID,
	})
	if err != nil {
		return err
	}
	_, err = c.purgeAccount(me, trimReason(reason))
	return err
}

func (c *AccountCore) purgeAccount(me *mtproto.UserData, reason string) (*mtproto.Bool, error) {
	if me.Username != "" {
		if _, err := c.svcCtx.Dao.UserClient.UserDeleteUsername(c.ctx, &user.TLUserDeleteUsername{
			Username: me.Username,
		}); err != nil {
			c.Logger.Errorf("account.deleteAccount - username: %v", err)
			return nil, err
		}
	}
	if _, err := c.svcCtx.UserClient.UserDeleteUser(c.ctx, &user.TLUserDeleteUser{
		UserId: c.MD.UserId,
		Reason: reason,
		Phone:  me.Phone,
	}); err != nil {
		c.Logger.Errorf("account.deleteAccount - delete: %v", err)
		return nil, err
	}
	tKeyIdList, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionResetAuthorization(c.ctx, &authsession.TLAuthsessionResetAuthorization{
		UserId:    c.MD.UserId,
		AuthKeyId: 0,
		Hash:      0,
	})
	if err != nil {
		c.Logger.Errorf("account.deleteAccount - sessions: %v", err)
		return nil, err
	}
	for _, id := range tKeyIdList.Datas {
		upds := mtproto.MakeTLUpdateAccountResetAuthorization(&mtproto.Updates{
			UserId:    c.MD.UserId,
			AuthKeyId: id,
		}).To_Updates()
		_, _ = c.svcCtx.Dao.SyncClient.SyncUpdatesMe(c.ctx, &sync.TLSyncUpdatesMe{
			UserId:        c.MD.UserId,
			PermAuthKeyId: id,
			Updates:       upds,
		})
	}
	_, _ = c.svcCtx.Dao.AuthsessionClient.AuthsessionUnbindAuthKeyUser(c.ctx, &authsession.TLAuthsessionUnbindAuthKeyUser{
		AuthKeyId: 0,
		UserId:    c.MD.UserId,
	})
	c.Logger.Infof("account.deleteAccount - deleted user=%d", c.MD.UserId)
	return mtproto.BoolTrue, nil
}

func (c *AccountCore) notifyDeletionScheduled() {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.SyncClient == nil || c.MD == nil {
		return
	}
	upd := mtproto.MakeTLUpdateServiceNotification(&mtproto.Update{
		Popup:          false,
		InboxDate:      mtproto.MakeFlagsInt32(int32(time.Now().Unix())),
		Type:           "ACCOUNT_DELETE_PENDING",
		Message_STRING: "你的 Tellomi 账号将在 7 天后删除。在此之前重新登录即可取消。",
		Media:          mtproto.MakeTLMessageMediaEmpty(nil).To_MessageMedia(),
	}).To_Update()
	_, _ = c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
		UserId:  c.MD.UserId,
		Updates: mtproto.MakeUpdatesByUpdates(upd),
	})
}

func (c *AccountCore) permAuthKey() int64 {
	if c == nil || c.MD == nil {
		return 0
	}
	if c.MD.PermAuthKeyId != 0 {
		return c.MD.PermAuthKeyId
	}
	return c.MD.AuthId
}

func srpProvided(p *mtproto.InputCheckPasswordSRP) bool {
	if p == nil {
		return false
	}
	if p.PredicateName == mtproto.Predicate_inputCheckPasswordSRP {
		return true
	}
	return p.Constructor == mtproto.CRC32_inputCheckPasswordSRP
}

func trimReason(reason string) string {
	if len(reason) > 128 {
		return reason[:128]
	}
	return reason
}
