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

// AccountAcceptAuthorization
// account.acceptAuthorization#f3ed4c73 bot_id:long scope:string public_key:string value_hashes:Vector<SecureValueHash> credentials:SecureCredentialsEncrypted = Bool;
// https://core.telegram.org/method/account.acceptAuthorization
func (c *PassportCore) AccountAcceptAuthorization(in *mtproto.TLAccountAcceptAuthorization) (*mtproto.Bool, error) {
	user, err := c.requireUser()
	if err != nil {
		c.Logger.Errorf("account.acceptAuthorization - error: %v", err)
		return nil, err
	}
	if _, _, err = c.loadBot(in.GetBotId()); err != nil {
		c.Logger.Errorf("account.acceptAuthorization - error: %v", err)
		return nil, err
	}
	if err = c.book().Accept(c.ctx, user, in.GetBotId(), in.GetScope(), in.GetPublicKey(), in.GetValueHashes(), in.GetCredentials()); err != nil {
		c.Logger.Errorf("account.acceptAuthorization - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
