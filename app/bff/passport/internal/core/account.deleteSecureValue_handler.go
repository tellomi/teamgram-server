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

// AccountDeleteSecureValue
// account.deleteSecureValue#b880bc4b types:Vector<SecureValueType> = Bool;
// https://core.telegram.org/method/account.deleteSecureValue
func (c *PassportCore) AccountDeleteSecureValue(in *mtproto.TLAccountDeleteSecureValue) (*mtproto.Bool, error) {
	user, err := c.requireUser()
	if err != nil {
		c.Logger.Errorf("account.deleteSecureValue - error: %v", err)
		return nil, err
	}
	if err = c.book().Delete(c.ctx, user, in.GetTypes()); err != nil {
		c.Logger.Errorf("account.deleteSecureValue - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
