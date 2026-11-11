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

package config

import (
	kafka "github.com/teamgram/marmota/pkg/mq"
	gokv "github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	KV gokv.KvConf `json:",optional"`

	UserClient    zrpc.RpcClientConf
	ChatClient    zrpc.RpcClientConf
	MsgClient     zrpc.RpcClientConf
	DialogClient  zrpc.RpcClientConf
	IdgenClient   zrpc.RpcClientConf
	MessageClient zrpc.RpcClientConf
	MediaClient   zrpc.RpcClientConf
	SyncClient    *kafka.KafkaProducerConf

	// ChannelMySQL is the teamgram database DSN already used by the other
	// BFF DAOs. The all-in-one bff process passes its ChannelMySQL so
	// pending webpage jobs share that same connection source.
	// TEAMGRAM_MYSQL_DSN, if already set, overrides it the same way other
	// BFFs do. This is not a new production env var.
	ChannelMySQL string `json:",optional"`
}
