package server

import (
	"flag"

	"github.com/teamgram/teamgram-server/app/bff/folders/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/server/grpc"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/configload"
	"github.com/teamgram/teamgram-server/pkg/redact"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

var configFile = flag.String("f", "etc/folders.yaml", "the config file")

type Server struct {
	grpcSrv *zrpc.RpcServer
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	configload.MustLoad(*configFile, &c)

	logx.Infov(redact.Value(c))
	ctx := svc.NewServiceContext(c)
	s.grpcSrv = grpc.New(ctx, c.RpcServerConf)

	go func() {
		go s.grpcSrv.Start()
	}()
	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	s.grpcSrv.Stop()
}
