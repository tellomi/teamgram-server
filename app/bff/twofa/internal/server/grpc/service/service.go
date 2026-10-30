package service

import (
	"github.com/teamgram/proto/mtproto"
)

// Service implements cloud-password RPCs. account.getTmpPassword is on
// RPCPayments, which the bff process registers on the payments service.
// This type still implements that method so the two-factor core stays the
// one place the temporary password is built. Both generated servers are
// embedded by value. Their testEmbeddedByValue methods share a name, so
// the outer type does not promote that check.
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
