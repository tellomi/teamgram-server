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
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/rest"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	MiniHttp rest.RestConf
	Minio    minio_util.MinioConfig
	IdGen    zrpc.RpcClientConf
	SSDB     kv.KvConf
	// Mysql is the teamgram DSN used for content-addressed media_blobs.
	// Empty keeps the historical one-object-per-upload behavior.
	Mysql sqlx.Config `json:",optional"`
}
