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
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// AccountGetAuthorizationForm
// account.getAuthorizationForm#a929597a bot_id:long scope:string public_key:string = account.AuthorizationForm;
// https://core.telegram.org/method/account.getAuthorizationForm
func (c *PassportCore) AccountGetAuthorizationForm(in *mtproto.TLAccountGetAuthorizationForm) (*mtproto.Account_AuthorizationForm, error) {
	user, err := c.requireUser()
	if err != nil {
		c.Logger.Errorf("account.getAuthorizationForm - error: %v", err)
		return nil, err
	}
	bot, policy, err := c.loadBot(in.GetBotId())
	if err != nil {
		c.Logger.Errorf("account.getAuthorizationForm - error: %v", err)
		return nil, err
	}
	form, err := c.book().AuthorizationForm(c.ctx, user, in.GetBotId(), in.GetScope(), in.GetPublicKey())
	if err != nil {
		c.Logger.Errorf("account.getAuthorizationForm - error: %v", err)
		return nil, err
	}
	out := mtproto.MakeTLAccountAuthorizationForm(&mtproto.Account_AuthorizationForm{
		RequiredTypes: form.Required,
		Values:        form.Values,
		Errors:        form.Errors,
		Users:         []*mtproto.User{bot},
	})
	if policy != "" {
		out.SetPrivacyPolicyUrl(wrapperspb.String(policy))
	}
	return out.To_Account_AuthorizationForm(), nil
}
