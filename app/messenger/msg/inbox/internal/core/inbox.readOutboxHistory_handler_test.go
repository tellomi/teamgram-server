// Copyright 2026 Teamgram Authors
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
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSkipPrivateSelfOutbox(t *testing.T) {
	if !skipPrivateSelfOutbox(mtproto.PEER_USER, 5, 5) {
		t.Fatal("private self read should be skipped")
	}
	if !skipPrivateSelfOutbox(mtproto.PEER_SELF, 5, 5) {
		t.Fatal("saved messages read should be skipped")
	}
	if skipPrivateSelfOutbox(mtproto.PEER_USER, 5, 9) {
		t.Fatal("a peer reading the outbox is a real receipt")
	}
	if skipPrivateSelfOutbox(mtproto.PEER_CHAT, 5, 5) {
		t.Fatal("a chat id that matches the user id is still a group")
	}
}
