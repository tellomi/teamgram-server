package twofa_helper

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/twofa/internal/core"
	"github.com/teamgram/teamgram-server/app/bff/twofa/internal/server/grpc/service"
)

func New() *service.Service {
	return service.New()
}

// TmpPassword builds account.getTmpPassword. RPCPayments is registered on
// the payments service, which cannot import this module's internal core.
func TmpPassword(ctx context.Context, in *mtproto.TLAccountGetTmpPassword) (*mtproto.Account_TmpPassword, error) {
	return core.New(ctx).AccountGetTmpPassword(in)
}
