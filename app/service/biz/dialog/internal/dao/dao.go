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
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/config"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
)

// Dao dao.
type Dao struct {
	*Mysql
	sqlc.CachedConn
	// InvalidateDelay waits before the second DelCache after a dialog
	// write. Zero means dialog.DefaultCacheInvalidateDelay. Tests set a
	// short delay so they can wait for the goroutine.
	InvalidateDelay time.Duration
}

// New new a dao and return.
func New(c config.Config) (dao *Dao) {
	db := mysqltls.NewMySQL(&c.Mysql)
	return &Dao{
		Mysql:      newMysqlDao(db),
		CachedConn: sqlc.NewConn(db, c.Cache),
	}
}
