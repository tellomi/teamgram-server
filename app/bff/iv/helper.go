package iv_helper

import "github.com/teamgram/teamgram-server/app/bff/iv/internal/server/grpc/service"

// New builds the iv RPC service.
func New() *service.Service { return service.New() }
