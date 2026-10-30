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
