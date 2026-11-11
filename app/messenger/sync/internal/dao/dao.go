/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dao

import (
	"github.com/teamgram/teamgram-server/pkg/bootstrap"
	"sync"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/config"
	authsession_client "github.com/teamgram/teamgram-server/app/service/authsession/client"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	status_client "github.com/teamgram/teamgram-server/app/service/status/client"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	*Mysql
	kv               kv.Store
	conf             *config.Config
	mu               sync.RWMutex
	sessionServers   map[string]SessionPusher
	useStreamSession bool
	idgen_client.IDGenClient2
	status_client.StatusClient
	authsession_client.AuthsessionClient
	chat_client.ChatClient
	PushClient sync_client.SyncClient
}

func New(c config.Config) *Dao {
	db := bootstrap.MustMySQL(&c.Mysql)
	d := &Dao{
		Mysql:             newMysqlDao(db),
		kv:                kv.NewStore(c.KV),
		conf:              &c,
		sessionServers:    make(map[string]SessionPusher),
		useStreamSession:  c.UseStreamSession,
		IDGenClient2:      idgen_client.NewIDGenClient2(zrpc.MustNewClient(c.IdgenClient)),
		StatusClient:      status_client.NewStatusClient(zrpc.MustNewClient(c.StatusClient)),
		AuthsessionClient: newAuthSessionClient(c.AuthSession),
		ChatClient:        chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient)),
	}
	if c.PushClient != nil {
		d.PushClient = sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(c.PushClient))
	}

	go d.watch(c.SessionClient)
	return d
}

// newAuthSessionClient dials authsession when the config names a target.
// An empty block leaves the client nil; push then keeps the previous route
// and the session process still checks the expected user before delivery.
func newAuthSessionClient(c zrpc.RpcClientConf) authsession_client.AuthsessionClient {
	if c.Etcd.Key == "" && len(c.Endpoints) == 0 && c.Target == "" {
		return nil
	}
	return authsession_client.NewAuthsessionClient(zrpc.MustNewClient(c))
}

// SetSessionPusher registers one session node. Discovery replaces the map
// when the etcd set changes; tests use this to observe a push.
func (d *Dao) SetSessionPusher(serverID string, p SessionPusher) {
	if d == nil || serverID == "" || p == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sessionServers == nil {
		d.sessionServers = make(map[string]SessionPusher)
	}
	d.sessionServers[serverID] = p
}
