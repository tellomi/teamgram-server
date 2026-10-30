package svc

import "github.com/teamgram/teamgram-server/app/bff/feed/internal/book"

// ServiceContext holds the process-local feed book.
type ServiceContext struct {
	Book *book.Book
}

// New returns a context bound to the shared book.
func New() *ServiceContext {
	return &ServiceContext{Book: book.Shared()}
}
