package grpc

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

func New(ctx *svc.ServiceContext, c zrpc.RpcServerConf) *zrpc.RpcServer {
	s, err := zrpc.NewServer(c, func(grpcServer *grpc.Server) {
		impl := service.New(ctx)
		mtproto.RegisterRPCFoldersServer(grpcServer, impl)
		mtproto.RegisterRPCFolderTagsServer(grpcServer, impl)
	})
	logx.Must(err)
	return s
}
