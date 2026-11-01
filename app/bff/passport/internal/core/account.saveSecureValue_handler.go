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

// AccountSaveSecureValue
// account.saveSecureValue#899fe31d value:InputSecureValue secure_secret_id:long = SecureValue;
// https://core.telegram.org/method/account.saveSecureValue
func (c *PassportCore) AccountSaveSecureValue(in *mtproto.TLAccountSaveSecureValue) (*mtproto.SecureValue, error) {
	user, err := c.requireUser()
	if err != nil {
		c.Logger.Errorf("account.saveSecureValue - error: %v", err)
		return nil, err
	}
	out, err := c.book().Save(c.ctx, user, in.GetSecureSecretId(), in.GetValue())
	if err != nil {
		c.Logger.Errorf("account.saveSecureValue - error: %v", err)
		return nil, err
	}
	return out, nil
}
