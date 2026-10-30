package config

import (
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	ChannelMySQL  string `json:",optional"`
	UserClient    zrpc.RpcClientConf
	ChatClient    zrpc.RpcClientConf `json:",optional"`
	MessageClient zrpc.RpcClientConf `json:",optional"`
	IdgenClient   zrpc.RpcClientConf `json:",optional"`
	DialogClient  zrpc.RpcClientConf
	MediaClient   zrpc.RpcClientConf
	SyncClient    *kafka.KafkaProducerConf
}
