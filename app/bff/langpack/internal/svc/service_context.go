package svc

import (
	"github.com/teamgram/teamgram-server/app/bff/langpack/internal/config"
	"github.com/teamgram/teamgram-server/pkg/langpack"
)

type ServiceContext struct {
	Config  config.Config
	Catalog *langpack.Catalog
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config:  c,
		Catalog: langpack.DefaultCatalog(),
	}
}
