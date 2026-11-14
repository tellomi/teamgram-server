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
	"github.com/teamgram/teamgram-server/pkg/privacypremium"
)

// UsersGetIsPremiumRequiredToContact
// users.getIsPremiumRequiredToContact#a622aa10 id:Vector<InputUser> = Vector<Bool>;
// The layer-232 replacement is users.getRequirementsToContact. This constructor
// still answers older clients with the same fully checked bit: boolTrue when
// requirementToContactPremium would be returned for that user.
// https://core.telegram.org/method/users.getIsPremiumRequiredToContact
// https://core.telegram.org/api/privacy
func (c *PrivacySettingsCore) UsersGetIsPremiumRequiredToContact(in *mtproto.TLUsersGetIsPremiumRequiredToContact) (*mtproto.Vector_Bool, error) {
	var ids []*mtproto.InputUser
	if in != nil {
		ids = in.GetId()
	}
	out := make([]*mtproto.Bool, 0, len(ids))
	for _, id := range ids {
		if id == nil {
			out = append(out, mtproto.BoolFalse)
			continue
		}
		blocked, err := c.premiumRequired(id)
		if err != nil {
			return nil, err
		}
		out = append(out, privacypremium.Bool(blocked))
	}
	return &mtproto.Vector_Bool{Datas: out}, nil
}
