package config

import (
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	StoryMySQL  string `json:",optional"`
	UserClient  zrpc.RpcClientConf
	MediaClient zrpc.RpcClientConf
	SyncClient  *kafka.KafkaProducerConf
}
