// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dao

import (
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/config"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	media_client "github.com/teamgram/teamgram-server/app/service/media/client"

	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	dfs_client.DfsClient
	media_client.MediaClient
	user_client.UserClient
}

func New(c config.Config) *Dao {
	d := &Dao{}
	// An empty client conf is the combined-BFF unit test and a process that
	// has not been given endpoints. GetCachedRpcClient panics in that case,
	// and a dial is not required to install story media hooks.
	if rpcConfigured(c.DfsClient) {
		d.DfsClient = dfs_client.NewDfsClient(rpcx.GetCachedRpcClient(c.DfsClient))
	}
	if rpcConfigured(c.MediaClient) {
		d.MediaClient = media_client.NewMediaClient(rpcx.GetCachedRpcClient(c.MediaClient))
	}
	if rpcConfigured(c.UserClient) {
		d.UserClient = user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient))
	}
	return d
}

func rpcConfigured(c zrpc.RpcClientConf) bool {
	return c.Etcd.Key != "" || len(c.Endpoints) > 0
}
