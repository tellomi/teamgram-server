/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package config

import (
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	NodeId   int64 // snowflake
	SeqIDGen kv.KvConf
	// Mysql is read at startup so sequence counters can be raised to MAX(id).
	// Production must set it. An empty DSN skips the heal.
	Mysql sqlx.Config `json:",optional"`
}
