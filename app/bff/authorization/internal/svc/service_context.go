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
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/logic"
	"github.com/teamgram/teamgram-server/app/bff/authorization/plugin"
	"github.com/teamgram/teamgram-server/pkg/code"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
	"github.com/teamgram/teamgram-server/pkg/verify"
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
	return &ServiceContext{
		Config:       c,
		Dao:          d,
		AuthLogic:    auth,
		Plugin:       plugin,
		Verify:       cfg,
		Limiter:      verify.NewLimiter(d.Cache(), cfg.Limits),
		EmailLimiter: verify.NewLimiter(d.Cache(), cfg.EmailLimits),
		EmailSender:  verify.NewEmailSender(cfg.Email),
	}
}
