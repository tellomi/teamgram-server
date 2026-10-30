package config

import (
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DialogClient zrpc.RpcClientConf
	UserClient   zrpc.RpcClientConf
	ChatClient   zrpc.RpcClientConf
	SyncClient   *kafka.KafkaProducerConf
}
