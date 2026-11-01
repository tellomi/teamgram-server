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

package dao

import (
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/contacts/book"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/config"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"
)

type Dao struct {
	user_client.UserClient
	chat_client.ChatClient
	dialog_client.DialogClient
	sync_client.SyncClient
	Tokens *TokenBook
	Book   *book.Book
	Saved  SavedBook
}

func New(c config.Config) *Dao {
	rel := book.New(c.KV)
	book.Install(rel)
	d := &Dao{
		UserClient: user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		ChatClient: chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient)),
		SyncClient: sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(c.SyncClient)),
		Tokens:     NewTokenBook(c.KV),
		Book:       rel,
		Saved:      OpenSaved(c.Mysql),
	}
	if c.DialogClient.Etcd.Key != "" || len(c.DialogClient.Endpoints) > 0 || c.DialogClient.Target != "" {
		d.DialogClient = dialog_client.NewDialogClient(rpcx.GetCachedRpcClient(c.DialogClient))
	}
	return d
}
