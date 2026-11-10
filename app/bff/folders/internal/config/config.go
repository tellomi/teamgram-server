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
	// IdgenClient and MysqlDSN persist updateFolderPeers on the user pts
	// sequence. Both are optional so a folders process without them still
	// pushes the folder move with pts 0.
	IdgenClient zrpc.RpcClientConf `json:",optional"`
	MysqlDSN    string             `json:",optional"`
}
