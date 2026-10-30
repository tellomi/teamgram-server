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
	"os"
	"strings"

	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/promo"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/userinfo"
)

type ServiceContext struct {
	Config   config.Config
	Promo    *promo.Store
	UserInfo *userinfo.Store
}

func NewServiceContext(c config.Config) *ServiceContext {
	dsn := strings.TrimSpace(c.MysqlDSN)
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		dsn = env
	}
	return &ServiceContext{
		Config:   c,
		Promo:    promo.Open(dsn),
		UserInfo: userinfo.Open(dsn),
	}
}
