// Copyright 2026 Tellomi Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0

package langpack_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/langpack/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/langpack/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/langpack/internal/svc"
)

type (
	Config = config.Config
)

func New(c Config) *service.Service {
	return service.New(svc.NewServiceContext(c))
}
