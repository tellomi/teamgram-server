package communities_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/communities/internal/book"
	"github.com/teamgram/teamgram-server/app/bff/communities/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/communities/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// New builds the communities RPC service.
func New() *service.Service {
	return service.New(svc.New())
}

// Start opens the community tables on the service MySQL DSN. An empty DSN
// keeps the in-memory book for local development. A configured DSN that
// cannot be opened is remembered: community RPCs return an error and the
// next call tries again. They do not write to an empty memory book.
func Start(dsn string) {
	if err := book.Shared().OpenMySQL(dsn); err != nil {
		logx.Errorf("communities: mysql open failed; RPCs return an error until it connects: %v", err)
	}
}
