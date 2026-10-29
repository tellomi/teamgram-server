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

package mysql_dao

import (
	"errors"
	"testing"

	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
)

func TestSameStoredMessageRejectsResetCollision(t *testing.T) {
	old := &dataobject.MessagesDO{
		UserId:           1001,
		UserMessageBoxId: 1,
		SenderUserId:     1001,
		PeerType:         2,
		PeerId:           1002,
		RandomId:         11,
		Message:          "evening",
		Date2:            1761747000,
	}
	// Box id 1 was issued again after the Redis flush. Same slot, different send.
	incoming := &dataobject.MessagesDO{
		UserId:           1001,
		UserMessageBoxId: 1,
		SenderUserId:     1001,
		PeerType:         2,
		PeerId:           1002,
		RandomId:         99,
		Message:          "morning",
		Date2:            1761770000,
	}
	if sameStoredMessage(old, incoming) {
		t.Fatal("a new message on a reused box id must not be treated as a replay")
	}
}

func TestSameStoredMessageAllowsExactReplay(t *testing.T) {
	row := &dataobject.MessagesDO{
		UserId:           1001,
		UserMessageBoxId: 52,
		SenderUserId:     1001,
		PeerType:         2,
		PeerId:           1002,
		RandomId:         99,
		Message:          "morning",
		Date2:            1761770000,
	}
	if !sameStoredMessage(row, row) {
		t.Fatal("retry of the same random id is a replay")
	}
}

func TestCollisionErrorIsDistinct(t *testing.T) {
	err := errors.Join(ErrMessageBoxCollision, errors.New("user_id=1001 box_id=1"))
	if !errors.Is(err, ErrMessageBoxCollision) {
		t.Fatal("callers must be able to see ErrMessageBoxCollision")
	}
}
