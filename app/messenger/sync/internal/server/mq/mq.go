// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package mq

import (
	"context"
	"encoding/json"

	"github.com/teamgram/teamgram-server/pkg/bootstrap"

	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/core"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// New new a grpc server.
func New(svcCtx *svc.ServiceContext, conf kafka.KafkaConsumerConf) *kafka.ConsumerGroup {
	s := bootstrap.MustKafkaConsumer(&conf)
	s.RegisterHandlers(
		conf.Topics[0],
		func(ctx context.Context, method, key string, value []byte) {
			logx.WithContext(ctx).Debugf("method: %s, key: %s", method, key)

			switch protoreflect.FullName(method) {
			case proto.MessageName((*sync.TLSyncUpdatesMe)(nil)):
				c := core.New(ctx, svcCtx)

				r := new(sync.TLSyncUpdatesMe)
				if err := json.Unmarshal(value, r); err != nil {
					c.Logger.Error(err.Error())
					return
				}
				c.Logger.Debugf("sync.updatesMe - request: %s", r)

				c.SyncUpdatesMe(r)
			case proto.MessageName((*sync.TLSyncUpdatesNotMe)(nil)):
				c := core.New(ctx, svcCtx)

				r := new(sync.TLSyncUpdatesNotMe)
				if err := json.Unmarshal(value, r); err != nil {
					c.Logger.Error(err.Error())
					return
				}
				c.Logger.Debugf("sync.updatesNotMe - request: %s", r)

				c.SyncUpdatesNotMe(r)
			case proto.MessageName((*sync.TLSyncPushUpdates)(nil)):
				c := core.New(ctx, svcCtx)

				r := new(sync.TLSyncPushUpdates)
				if err := json.Unmarshal(value, r); err != nil {
					c.Logger.Error(err.Error())
					return
				}
				c.Logger.Debugf("sync.pushUpdates - request: %s", r)

				c.SyncPushUpdates(r)
			case proto.MessageName((*sync.TLSyncPushUpdatesIfNot)(nil)):
				c := core.New(ctx, svcCtx)

				r := new(sync.TLSyncPushUpdatesIfNot)
				if err := json.Unmarshal(value, r); err != nil {
					c.Logger.Error(err.Error())
					return
				}
				c.Logger.Debugf("sync.pushUpdatesIfNot - request: %s", r)

				c.SyncPushUpdatesIfNot(r)
			case proto.MessageName((*sync.TLSyncPushBotUpdates)(nil)):
				c := core.New(ctx, svcCtx)

				r := new(sync.TLSyncPushBotUpdates)
				if err := json.Unmarshal(value, r); err != nil {
					c.Logger.Error(err.Error())
					return
				}
				c.Logger.Debugf("sync.pushBotUpdates - request: %s", r)

				c.SyncPushBotUpdates(r)
			case proto.MessageName((*sync.TLSyncPushRpcResult)(nil)):
				c := core.New(ctx, svcCtx)

				r := new(sync.TLSyncPushRpcResult)
				if err := json.Unmarshal(value, r); err != nil {
					c.Logger.Error(err.Error())
					return
				}
				c.Logger.Debugf("sync.pushRpcResult - request: %s", r)

				c.SyncPushRpcResult(r)
			case proto.MessageName((*sync.TLSyncBroadcastUpdates)(nil)):
				c := core.New(ctx, svcCtx)

				r := new(sync.TLSyncBroadcastUpdates)
				if err := json.Unmarshal(value, r); err != nil {
					c.Logger.Error(err.Error())
					return
				}
				c.Logger.Debugf("sync.broadcastUpdates - request: %s", r)

				c.SyncBroadcastUpdates(r)
			default:
				logx.Errorf("sync mq: unhandled method %q key %s", method, key)
			}
		})
	return s
}
