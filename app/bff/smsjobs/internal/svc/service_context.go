package svc

import "github.com/teamgram/teamgram-server/app/bff/smsjobs/internal/book"

// ServiceContext holds the process-local SMS-jobs ledger.
type ServiceContext struct {
	Book *book.Book
}

// New returns a context bound to the shared closed ledger.
func New() *ServiceContext {
	return &ServiceContext{Book: book.Shared()}
}
