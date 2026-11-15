// Copyright 2024 Teamgram Authors
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
	"github.com/teamgram/teamgram-server/pkg/accountprefs"
)

// AccountToggleSponsoredMessages
// account.toggleSponsoredMessages#b9d9a38d enabled:Bool = Bool;
func (c *SponsoredMessagesCore) AccountToggleSponsoredMessages(in *mtproto.TLAccountToggleSponsoredMessages) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	enabled := mtproto.FromBool(in.GetEnabled())
	if err := accountprefs.SetSponsored(c.ctx, c.MD.UserId, enabled); err != nil {
		c.Logger.Errorf("account.toggleSponsoredMessages - store: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.BoolTrue, nil
}
