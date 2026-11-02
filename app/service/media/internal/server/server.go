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

	"github.com/teamgram/teamgram-server/app/service/media/internal/config"
	"github.com/teamgram/teamgram-server/app/service/media/internal/server/grpc"
	"github.com/teamgram/teamgram-server/app/service/media/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/configload"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

var configFile = flag.String("f", "etc/media.yaml", "the config file")

type Server struct {
	grpcSrv       *zrpc.RpcServer
	stopRetention func()
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	configload.MustLoad(*configFile, &c)

	logx.Infov(c)
	ctx := svc.NewServiceContext(c)
	s.grpcSrv = grpc.New(ctx, c.RpcServerConf)

	go func() {
		go s.grpcSrv.Start()
	}()
	s.stopRetention = startRetention(c)
	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	if s.stopRetention != nil {
		s.stopRetention()
	}
	s.grpcSrv.Stop()
}
