package svc

import "github.com/teamgram/teamgram-server/app/bff/business/profile"

// ServiceContext holds the process-local business profile book.
type ServiceContext struct {
	Book *profile.Book
}

// New returns a context bound to the shared book.
func New() *ServiceContext {
	return &ServiceContext{Book: profile.Shared()}
}
