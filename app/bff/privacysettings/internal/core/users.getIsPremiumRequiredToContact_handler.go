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
)

// UsersGetIsPremiumRequiredToContact
// users.getIsPremiumRequiredToContact#a622aa10 id:Vector<InputUser> = Vector<Bool>;
// UsersGetIsPremiumRequiredToContact reports whether each user requires a
// premium subscription before they can be contacted. Tellomi does not gate
// contact on premium, so every id is boolFalse. An empty id list is an empty
// vector. https://core.telegram.org/method/users.getIsPremiumRequiredToContact
func (c *PrivacySettingsCore) UsersGetIsPremiumRequiredToContact(in *mtproto.TLUsersGetIsPremiumRequiredToContact) (*mtproto.Vector_Bool, error) {
	out := make([]*mtproto.Bool, 0, len(in.GetId()))
	for _, id := range in.GetId() {
		if id == nil {
			continue
		}
		out = append(out, mtproto.BoolFalse)
	}
	return &mtproto.Vector_Bool{Datas: out}, nil
}
