/*
 * WARNING! All changes made in this file will be lost!
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright 2022 Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package messages_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/core"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/messages/plugin"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
)

type (
	Config = config.Config
)

func New(c Config, plugin plugin.MessagesPlugin) *service.Service {
	svcCtx := svc.NewServiceContext(c, plugin)
	if svcCtx.Dao != nil {
		var chats chat_client.ChatClient
		if svcCtx.Dao.ChatClient != nil {
			chats = svcCtx.Dao.ChatClient.Client()
		}
		BindPreview(svcCtx.Dao.UserClient, svcCtx.Dao.MediaClient, chats)
	}
	// The all-in-one bff process starts messages through this helper, not
	// the standalone messages server, so the pending-job store and sweeper
	// must start here. ChannelMySQL is already on c.
	core.StartPendingWebpageSweeper(svcCtx)
	return service.New(svcCtx)
}
