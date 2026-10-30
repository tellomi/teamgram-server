package service

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/svc"
)

// Service implements the folder RPCs from layer 232.
// Both generated servers are embedded by value. Their testEmbeddedByValue
// methods share a name, so the outer type does not promote that check.
type Service struct {
	mtproto.UnimplementedRPCFoldersServer
	mtproto.UnimplementedRPCFolderTagsServer
	svcCtx *svc.ServiceContext
}

func New(ctx *svc.ServiceContext) *Service {
	return &Service{svcCtx: ctx}
}

func (s *Service) GetServiceContext() *svc.ServiceContext {
	return s.svcCtx
}

var (
	_ mtproto.RPCFoldersServer    = (*Service)(nil)
	_ mtproto.RPCFolderTagsServer = (*Service)(nil)
)
