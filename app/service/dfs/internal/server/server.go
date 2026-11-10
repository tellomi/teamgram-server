/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package server

import (
	"flag"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/server/http"
	"github.com/zeromicro/go-zero/rest"

	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/config"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/server/grpc"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/configload"
	"github.com/teamgram/teamgram-server/pkg/redact"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

var configFile = flag.String("f", "etc/dfs.yaml", "the config file")

type Server struct {
	grpcSrv *zrpc.RpcServer
	httpSrv *rest.Server
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	configload.MustLoad(*configFile, &c)
	if err := c.Minio.ApplyEnv(); err != nil {
		logx.Errorf("object storage config: %v", err)
		return err
	}
	logx.Infov(redact.Value(c))
	// Official sticker and emoji ids, so downloadFile can skip the documents
	// bucket. A failure here leaves the built-in index and the old fallback.
	catalog.WatchPublicObjects(c.Mysql.DSN)
	ctx := svc.NewServiceContext(c)

	s.grpcSrv = grpc.New(ctx, c.RpcServerConf)
	go func() {
		s.grpcSrv.Start()
	}()

	s.httpSrv = http.New(ctx, c.MiniHttp)

	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	s.grpcSrv.Stop()
}
