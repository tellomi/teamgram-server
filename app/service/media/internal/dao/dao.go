/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dao

import (
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	"github.com/teamgram/teamgram-server/app/service/media/internal/config"
	"github.com/teamgram/teamgram-server/app/service/mediablob"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	*Mysql
	sqlc.CachedConn
	dfs_client.DfsClient
	Blobs *mediablob.Store
}

func New(c config.Config) *Dao {
	db := sqlx.NewMySQL(&c.Mysql)
	blobs, err := mediablob.Open(c.Mysql.DSN)
	if err != nil {
		logx.Errorf("media: content-addressed blobs disabled: %v", err)
	}
	return &Dao{
		Mysql:      newMysqlDao(db),
		CachedConn: sqlc.NewConn(db, c.Cache),
		DfsClient:  dfs_client.NewDfsClient(zrpc.MustNewClient(c.Dfs)),
		Blobs:      blobs,
	}
}
