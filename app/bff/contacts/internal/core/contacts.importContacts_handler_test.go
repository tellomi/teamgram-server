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

package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestContactsImportContactsRequiresUser(t *testing.T) {
	c := &ContactsCore{Logger: logx.WithContext(context.Background())}
	if _, err := c.ContactsImportContacts(&mtproto.TLContactsImportContacts{}); err == nil {
		t.Fatal("expected an error without a user id")
	}
}

func TestContactsImportContactsEmpty(t *testing.T) {
	c := &ContactsCore{MD: &metadata.RpcMetadata{UserId: 7}}
	out, err := c.ContactsImportContacts(&mtproto.TLContactsImportContacts{})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out.Imported) != 0 || len(out.Users) != 0 {
		t.Fatalf("empty import = %#v", out)
	}
}

func TestImportPeerSettingsNamesTheUser(t *testing.T) {
	update := importPeerSettings(42)
	if update.GetPeer_PEER().GetUserId() != 42 {
		t.Fatalf("peer = %#v", update.GetPeer_PEER())
	}
	if update.GetSettings() == nil {
		t.Fatal("missing peer settings")
	}
}
