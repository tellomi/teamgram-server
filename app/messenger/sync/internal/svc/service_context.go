/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package svc

import (
	"os"
	"strings"

	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/config"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
	"github.com/teamgram/teamgram-server/pkg/push"
	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
	Push *push.Service
}

func NewServiceContext(c config.Config) *ServiceContext {
	sc := &ServiceContext{
		Config: c,
		Dao:    dao.New(c),
	}
	sc.Push = newPushService(c.Mysql.DSN)
	return sc
}

func newPushService(dsn string) *push.Service {
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		dsn = env
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		logx.Errorf("push: sync has no MySQL DSN")
		return nil
	}
	store, err := push.Open(dsn)
	if err != nil {
		logx.Errorf("push: open device store: %v", err)
		return nil
	}
	return push.NewService(store, push.LoadConfig())
}
