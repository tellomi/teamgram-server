/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"context"

	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/uploadledger"

	"github.com/zeromicro/go-zero/core/logx"
)

type DfsCore struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	MD *metadata.RpcMetadata
	// idHook allocates document and photo ids in tests. Production leaves it nil.
	idHook func() int64
}

// ledgerSubject is the uploader the files service charged. A logged-in
// request carries UserId in RPC metadata; the DFS creator field is the
// permanent auth key and is only the subject when nobody is logged in.
func (c *DfsCore) ledgerSubject(creator int64) int64 {
	if c != nil && c.MD != nil && c.MD.UserId != 0 {
		return c.MD.UserId
	}
	if c != nil {
		if id := uploadledger.UserFrom(c.ctx); id != 0 {
			return id
		}
	}
	return creator
}

func (c *DfsCore) ledgerSubjectFrom(ctx context.Context, creator int64) int64 {
	if c != nil && c.MD != nil && c.MD.UserId != 0 {
		return c.MD.UserId
	}
	if id := uploadledger.UserFrom(ctx); id != 0 {
		return id
	}
	return c.ledgerSubject(creator)
}

func (c *DfsCore) nextID() int64 {
	if c != nil && c.idHook != nil {
		return c.idHook()
	}
	return c.svcCtx.Dao.IDGenClient2.NextId(c.ctx)
}

func New(ctx context.Context, svcCtx *svc.ServiceContext) *DfsCore {
	return &DfsCore{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		MD:     metadata.RpcMetadataFromIncoming(ctx),
	}
}
