package svc

import "github.com/teamgram/teamgram-server/app/bff/ephemeral/internal/box"

// ServiceContext holds the process-local ephemeral box.
type ServiceContext struct {
	Box *box.Box
}

// New returns a context bound to the shared box.
func New() *ServiceContext {
	return &ServiceContext{Box: box.Shared()}
}
