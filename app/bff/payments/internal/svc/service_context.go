package svc

import "github.com/teamgram/teamgram-server/app/bff/payments/internal/book"

// ServiceContext is the Stars and giveaway book for this process.
type ServiceContext struct {
	Book *book.Book
}

// New returns the process-wide book.
func New() *ServiceContext {
	return &ServiceContext{Book: book.Shared()}
}
