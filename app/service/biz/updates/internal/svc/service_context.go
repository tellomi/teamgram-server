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
	"github.com/teamgram/teamgram-server/app/bff/phone/secretchat"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/config"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/dao"

	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
}

func NewServiceContext(c config.Config) *ServiceContext {
	if err := secretchat.UseDSN(c.Mysql.DSN); err != nil {
		logx.Errorf("secret qts mysql: %v", err)
	}
	return &ServiceContext{
		Config: c,
		Dao:    dao.New(c),
	}
}
