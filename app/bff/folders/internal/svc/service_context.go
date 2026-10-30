package svc

import (
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/dao"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config: c,
		Dao:    dao.New(c),
	}
}
