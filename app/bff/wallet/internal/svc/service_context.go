package svc

import "github.com/teamgram/teamgram-server/app/bff/wallet/internal/ledger"

// ServiceContext holds the process-local wallet book.
type ServiceContext struct {
	Ledger *ledger.Ledger
}

// New returns a context with an empty ledger.
func New() *ServiceContext {
	return &ServiceContext{Ledger: ledger.New()}
}
