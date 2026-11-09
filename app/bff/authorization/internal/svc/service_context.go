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

package svc

import (
	"context"
	"os"
	"strings"

	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/logic"
	"github.com/teamgram/teamgram-server/app/bff/authorization/plugin"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
	"github.com/teamgram/teamgram-server/pkg/code"
	"github.com/teamgram/teamgram-server/pkg/enforcement"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
	"github.com/teamgram/teamgram-server/pkg/push"
	"github.com/teamgram/teamgram-server/pkg/verify"

	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
	*logic.AuthLogic
	Plugin       plugin.AuthorizationPlugin
	Verify       verify.Config
	Limiter      *verify.Limiter
	EmailLimiter *verify.Limiter
	EmailSender  verify.CodeSender
	Devices      push.Store
}

func NewServiceContext(c config.Config, code2 code.VerifyCodeInterface, plugin plugin.AuthorizationPlugin) *ServiceContext {
	phonenumber.SetTestPhoneRanges(c.TestPhones)
	d := dao.New(c)
	if code2 == nil {
		code2 = code.NewVerifyCode(c.Code)
	}
	cfg := verify.Load()
	auth := logic.NewAuthSignLogic(d, code2)
	auth.MaxAttempts = cfg.Limits.MaxAttempts
	auth.EmailMaxAttempts = cfg.EmailLimits.MaxAttempts
	smsLim := verify.NewLimiter(d.Cache(), cfg.Limits)
	emailLim := verify.NewLimiter(d.Cache(), cfg.EmailLimits)
	verify.UseSharedLimiters(smsLim, emailLim)
	var devices push.Store
	if opened, err := push.OpenConfigured(c.MysqlDSN); err != nil {
		logx.Errorf("push: open device store: %v", err)
	} else if opened != nil {
		devices = opened
	}
	if svc := enforcement.InstallFromEnv(); svc != nil {
		svc.SetSender(enforcement.AdminSender{Push: d.MsgClient})
		svc.SetRevoker(func(userID int64) error {
			return revokeUserSessions(context.Background(), d.AuthsessionClient, devices, userID)
		})
		if os.Getenv(enforcement.EnvE2E) == "1" {
			go func() {
				if err := enforcement.ServeIsolatedE2E(enforcement.E2ELoopbackAddr); err != nil {
					logx.Error("enforcement: isolated e2e listener was not started")
				}
			}()
		}
		if addr := strings.TrimSpace(os.Getenv(enforcement.EnvAddr)); addr != "" && addr != "off" {
			go func() {
				if err := enforcement.ServeNetwork(addr); err != nil {
					logx.Error("enforcement: network listener was not started")
				}
			}()
		}
	}
	return &ServiceContext{
		Config:       c,
		Dao:          d,
		AuthLogic:    auth,
		Plugin:       plugin,
		Verify:       cfg,
		Limiter:      smsLim,
		EmailLimiter: emailLim,
		EmailSender:  verify.NewEmailSender(cfg.Email),
		Devices:      devices,
	}
}

// revokeUserSessions ends every login of userID and drops that user's push
// registrations. Enforcement calls it. auth key 0 is "exclude nobody".
// A device-store error is logged and does not fail the revocation: the
// sessions are already reset, and ListByUser skips a logged-out key.
func revokeUserSessions(ctx context.Context, client authsessionclient.AuthsessionClient, devices push.Store, userID int64) error {
	_, err := client.AuthsessionResetAuthorization(ctx, &authsession.TLAuthsessionResetAuthorization{
		UserId:    userID,
		AuthKeyId: 0,
		Hash:      0,
	})
	if err != nil {
		return err
	}
	if uerr := push.UnregisterSession(ctx, devices, userID, 0); uerr != nil {
		logx.WithContext(ctx).Errorf("push: unregister devices user=%d: %v", userID, uerr)
	}
	return nil
}
