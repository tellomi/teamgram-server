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
	"context"
	"flag"

	"github.com/teamgram/teamgram-server/app/service/idgen/internal/config"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/seqfloor"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/server/grpc"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

var configFile = flag.String("f", "etc/idgen.yaml", "the config file")

type Server struct {
	grpcSrv *zrpc.RpcServer
	floors  *seqfloor.Cache
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	conf.MustLoad(*configFile, &c)

	logx.Infov(c)
	ctx := svc.NewServiceContext(c)
	floors, err := seqfloor.Heal(context.Background(), c.Mysql.DSN, ctx.Dao.KV)
	if err != nil {
		return err
	}
	ctx.Dao.Floors = floors
	s.floors = floors
	s.grpcSrv = grpc.New(ctx, c.RpcServerConf)

	go func() {
		s.grpcSrv.Start()
	}()
	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	s.grpcSrv.Stop()
	if s.floors != nil {
		_ = s.floors.Close()
	}
}
