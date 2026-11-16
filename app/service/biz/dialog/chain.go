package dialog_helper

import (
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/core"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

// ServiceContext, NewDao, and NewDialogCore let an in-process caller run the
// dialog handler against a database it opened. The gRPC server uses the same
// core.
type ServiceContext = svc.ServiceContext

var (
	NewDao        = dao.New
	NewDialogCore = core.New
)
