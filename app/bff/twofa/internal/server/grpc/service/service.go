package service

import (
	"github.com/teamgram/proto/mtproto"
)

// Service implements cloud-password RPCs and account.getTmpPassword.
// Both generated servers are embedded by value. Their testEmbeddedByValue
// methods share a name, so the outer type does not promote that check.
// Other RPCPayments methods stay unimplemented and fail as METHOD_NOT_IMPL.
type Service struct {
	mtproto.UnimplementedRPCTwoFaServer
	mtproto.UnimplementedRPCPaymentsServer
}

func New() *Service {
	return &Service{}
}

var (
	_ mtproto.RPCTwoFaServer    = (*Service)(nil)
	_ mtproto.RPCPaymentsServer = (*Service)(nil)
)
