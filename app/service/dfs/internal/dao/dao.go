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
	"context"
	"fmt"

	"github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/config"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/teamgram/teamgram-server/app/service/mediablob"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	*minio_util.MinioUtil
	idgen_client.IDGenClient2
	ssdb  kv.ExtStore
	Blobs *mediablob.Store
}

func New(c config.Config) *Dao {
	blobs, err := mediablob.Open(c.Mysql.DSN)
	if err != nil {
		logx.Errorf("dfs: content-addressed blobs disabled: %v", err)
	}
	return &Dao{
		MinioUtil:    minio_util.MustNewMinioClient(&c.Minio),
		IDGenClient2: idgen_client.NewIDGenClient2(zrpc.MustNewClient(c.IdGen)),
		ssdb:         kv.NewStore(c.SSDB),
		Blobs:        blobs,
	}
}

func NewDFSHelper(minio *minio_util.MinioConfig, idgen zrpc.RpcClientConf, ssdb kv.KvConf) *Dao {
	return &Dao{
		MinioUtil:    minio_util.MustNewMinioClient(minio),
		IDGenClient2: idgen_client.NewIDGenClient2(zrpc.MustNewClient(idgen)),
		ssdb:         kv.NewStore(ssdb),
	}
}

// NewDaoWithStore builds a Dao around an existing part cache. Tests use it
// to exercise upload finalize without a live Redis.
func NewDaoWithStore(store kv.ExtStore) *Dao {
	return &Dao{ssdb: store}
}

func NewMinioHelper(minio *minio_util.MinioConfig) *minio_util.MinioUtil {
	return minio_util.MustNewMinioClient(minio)
}

// DocumentObjectPath is the stored object key. A reused document points at
// the first upload's key; everyone else stays at <id>.dat.
func (d *Dao) DocumentObjectPath(ctx context.Context, id int64) string {
	def := fmt.Sprintf("%d.dat", id)
	if d == nil || d.Blobs == nil {
		return def
	}
	p, err := d.Blobs.DocumentPath(ctx, id)
	if err != nil || p == "" {
		return def
	}
	return p
}

// PhotoObjectPath is the stored key for one photo size.
func (d *Dao) PhotoObjectPath(ctx context.Context, id int64, sizeType string) string {
	def := fmt.Sprintf("%s/%d.dat", sizeType, id)
	if d == nil || d.Blobs == nil || sizeType == "" {
		return def
	}
	p, err := d.Blobs.PhotoPath(ctx, id, sizeType)
	if err != nil || p == "" {
		return def
	}
	return p
}
