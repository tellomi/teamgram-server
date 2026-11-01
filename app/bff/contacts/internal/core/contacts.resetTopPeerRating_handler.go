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
	"github.com/teamgram/teamgram-server/app/bff/contacts/book"
)

// ContactsResetTopPeerRating
// contacts.resetTopPeerRating#1ae373ac category:TopPeerCategory peer:InputPeer = Bool;
// https://core.telegram.org/method/contacts.resetTopPeerRating
//
// Forces the peer's rating in that category to zero until the next recorded use.
func (c *ContactsCore) ContactsResetTopPeerRating(in *mtproto.TLContactsResetTopPeerRating) (*mtproto.Bool, error) {
	if err := c.requireSelf(); err != nil {
		return nil, err
	}
	cat := book.CategoryOf(in.GetCategory())
	if cat == "" {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if peer == nil || peer.IsEmpty() || peer.PeerId == 0 || !(peer.IsUser() || peer.IsChat() || peer.IsChannel()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err := c.relBook().ResetTop(c.ctx, c.MD.UserId, cat, book.Peer{Type: peer.PeerType, ID: peer.PeerId}); err != nil {
		c.Logger.Errorf("contacts.resetTopPeerRating - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
