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

package svc

import (
	"time"

	"github.com/teamgram/teamgram-server/app/bff/files/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/files/plugin"
	"github.com/teamgram/teamgram-server/app/service/media/retention"
	"github.com/teamgram/teamgram-server/app/service/mediablob"
	"github.com/teamgram/teamgram-server/pkg/mediaaccess"

	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
	Plugin1 plugin.ThumbFilesPlugin
	Plugin2 plugin.StreamFilesPlugin
	// Retention reports media removed by the retention sweeper. Nil when no
	// DSN is configured.
	Retention *retention.Checker
	// Blobs looks up documents by sha256 for messages.getDocumentByHash.
	Blobs *mediablob.Store
	// Access checks encrypted files, peer photos, and whether the caller
	// already has a document. Nil when Mysql is unset.
	Access *mediaaccess.Store
}

func NewServiceContext(c config.Config, plugin1 plugin.ThumbFilesPlugin, plugin2 plugin.StreamFilesPlugin) *ServiceContext {
	svcCtx := &ServiceContext{
		Config:  c,
		Dao:     dao.New(c),
		Plugin1: plugin1,
		Plugin2: plugin2,
	}
	if c.Mysql != "" {
		store, err := retention.OpenSQL(c.Mysql)
		if err != nil {
			logx.Errorf("files: media retention check disabled: %v", err)
		} else {
			svcCtx.Retention = retention.NewChecker(store, time.Minute)
		}
		blobs, err := mediablob.Open(c.Mysql)
		if err != nil {
			logx.Errorf("files: messages.getDocumentByHash lookup disabled: %v", err)
		} else {
			svcCtx.Blobs = blobs
		}
		access, err := mediaaccess.Open(c.Mysql)
		if err != nil {
			logx.Errorf("files: media access checks disabled: %v", err)
		} else {
			svcCtx.Access = access
		}
	}
	return svcCtx
}
