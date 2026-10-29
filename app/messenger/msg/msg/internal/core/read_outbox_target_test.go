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

	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
)

func TestChooseOutboxReadMessage(t *testing.T) {
	const reader int64 = 7
	own := &dataobject.MessagesDO{SenderUserId: reader, DialogMessageId: 30, UserMessageBoxId: 12}
	other := &dataobject.MessagesDO{SenderUserId: 9, DialogMessageId: 20, UserMessageBoxId: 11}
	older := &dataobject.MessagesDO{SenderUserId: 8, DialogMessageId: 10, UserMessageBoxId: 4}

	if got := chooseOutboxReadMessage(reader, nil, other); got != nil {
		t.Fatalf("nil max message = %#v", got)
	}
	if got := chooseOutboxReadMessage(reader, &dataobject.MessagesDO{}, other); got != nil {
		t.Fatalf("sender 0 = %#v", got)
	}
	if got := chooseOutboxReadMessage(reader, other, nil); got != other {
		t.Fatalf("other sender = %#v", got)
	}
	if got := chooseOutboxReadMessage(reader, own, older); got != older {
		t.Fatalf("own max should use newest other, got %#v", got)
	}
	if got := chooseOutboxReadMessage(reader, own, nil); got != nil {
		t.Fatalf("own max with no other message = %#v", got)
	}
	if got := chooseOutboxReadMessage(reader, own, own); got != nil {
		t.Fatalf("other message from self = %#v", got)
	}
	if got := chooseOutboxReadMessage(reader, own, &dataobject.MessagesDO{DialogMessageId: 1}); got != nil {
		t.Fatalf("other message with sender 0 = %#v", got)
	}
}
