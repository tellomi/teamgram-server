package core

import (
	"context"

	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/core/logx"
)

type TwoFaCore struct {
	ctx context.Context
	logx.Logger
	MD *metadata.RpcMetadata
}

func New(ctx context.Context) *TwoFaCore {
	return &TwoFaCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     metadata.RpcMetadataFromIncoming(ctx),
	}
}

func (c *TwoFaCore) userID() int64 {
	if c == nil || c.MD == nil {
		return 0
	}
	return c.MD.UserId
}

func (c *TwoFaCore) authKey() int64 {
	if c == nil || c.MD == nil {
		return 0
	}
	return c.MD.PermAuthKeyId
}
