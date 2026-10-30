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

	"github.com/teamgram/teamgram-server/app/bff/notification/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/reactions"
	"github.com/teamgram/teamgram-server/app/bff/notification/plugin"
	"github.com/teamgram/teamgram-server/pkg/push"
	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
	Plugin    plugin.NotificationPlugin
	Devices   push.Store
	Reactions reactions.Store
}

func NewServiceContext(c config.Config, plugin plugin.NotificationPlugin) *ServiceContext {
	sc := &ServiceContext{
		Config: c,
		Dao:    dao.New(c),
		Plugin: plugin,
	}
	dsn := strings.TrimSpace(c.MysqlDSN)
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		dsn = env
	}
	sc.Reactions = reactions.Open(dsn)
	if dsn == "" {
		logx.Errorf("push: no MySQL DSN; account.registerDevice will fail")
		return sc
	}
	store, err := push.Open(dsn)
	if err != nil {
		logx.Errorf("push: open device store: %v", err)
		return sc
	}
	sc.Devices = store
	return sc
}
