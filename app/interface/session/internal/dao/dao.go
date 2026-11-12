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
	"context"
	"sync"

	"github.com/teamgram/marmota/pkg/cache"
	"github.com/teamgram/marmota/pkg/net/ip"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	bff_proxy_client "github.com/teamgram/teamgram-server/app/bff/bff/client"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/config"
	authsession_client "github.com/teamgram/teamgram-server/app/service/authsession/client"
	status_client "github.com/teamgram/teamgram-server/app/service/status/client"
	"github.com/teamgram/teamgram-server/pkg/flood"

	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	cache *cache.LRUCache
	authsession_client.AuthsessionClient
	status_client.StatusClient
	*bff_proxy_client.BFFProxyClient
	gateMu           sync.RWMutex
	eGateServers     map[string]*Gateway
	MyServerId       string
	UseStreamGateway bool
	streamingGateway *StreamingGateway
	*RpcShardingManager
	Flood *flood.Limiter
}

func New(c config.Config) *Dao {
	myServerId := ip.FigureOutListenOn(c.ListenOn)
	d := &Dao{
		cache:              cache.NewLRUCache(1024 * 1024 * 1024),
		AuthsessionClient:  authsession_client.NewAuthsessionClient(zrpc.MustNewClient(c.AuthSession)),
		BFFProxyClient:     bff_proxy_client.NewBFFProxyClients(c.BFFProxyClients.Clients, c.BFFProxyClients.IDMap),
		StatusClient:       status_client.NewStatusClient(zrpc.MustNewClient(c.StatusClient)),
		eGateServers:       make(map[string]*Gateway),
		MyServerId:         myServerId,
		UseStreamGateway:   c.UseStreamGateway,
		RpcShardingManager: NewRpcShardingManager(myServerId, c.Etcd),
	}

	if c.UseStreamGateway {
		d.streamingGateway = NewStreamingGateway()
	}

	d.watchGateway(c.GatewayClient)
	d.Flood = openFlood(c)

	return d
}

// NewDirect is the session dao used when gnetway and session run in one
// process. It does not dial etcd. Production startup uses New.
func NewDirect(auth authsession_client.AuthsessionClient, lim *flood.Limiter) *Dao {
	return &Dao{
		cache:             cache.NewLRUCache(1024 * 1024),
		AuthsessionClient: auth,
		eGateServers:      map[string]*Gateway{},
		MyServerId:        "direct",
		Flood:             lim,
	}
}

func (d *Dao) InvokeContext(ctx context.Context, rpcMetaData *metadata.RpcMetadata, object mtproto.TLObject) (mtproto.TLObject, error) {
	return d.BFFProxyClient.InvokeContext(ctx, rpcMetaData, object)
}
