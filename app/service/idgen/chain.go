package idgen_helper

import (
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/core"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/svc"
)

// Dao, ServiceContext, and NewCore let an in-process caller allocate sequence
// values through the idgen core. The gRPC server uses the same core.
type (
	Dao            = dao.Dao
	ServiceContext = svc.ServiceContext
	IdgenCore      = core.IdgenCore
)

var NewCore = core.New
