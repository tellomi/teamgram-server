package config

import (
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	ChannelMySQL string `json:",optional"`
	UserClient   zrpc.RpcClientConf
	DialogClient zrpc.RpcClientConf
	SyncClient   *kafka.KafkaProducerConf
}
