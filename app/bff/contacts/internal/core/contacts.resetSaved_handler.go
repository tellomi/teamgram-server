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
	"github.com/teamgram/proto/mtproto"
)

// ContactsResetSaved
// contacts.resetSaved#879537f1 = Bool;
// https://core.telegram.org/method/contacts.resetSaved
//
// Removes every saved phone-book row that has no Telegram account.
// Matched contacts are left in place. Clients call this during startup, so a
// store error is logged and the RPC still succeeds.
func (c *ContactsCore) ContactsResetSaved(in *mtproto.TLContactsResetSaved) (*mtproto.Bool, error) {
	_ = in
	if err := c.requireSelf(); err != nil {
		return nil, err
	}
	if store := c.savedBook(); store != nil {
		if err := store.ResetUnregistered(c.ctx, c.MD.UserId); err != nil {
			c.Logger.Errorf("contacts.resetSaved - error: %v", err)
		}
	}
	return mtproto.BoolTrue, nil
}
