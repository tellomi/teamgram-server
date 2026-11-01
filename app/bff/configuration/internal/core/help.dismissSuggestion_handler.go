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

// HelpDismissSuggestion
// help.dismissSuggestion#f50dbaa1 peer:InputPeer suggestion:string = Bool;
// https://core.telegram.org/method/help.dismissSuggestion
// https://core.telegram.org/api/config#suggestions
//
// Remembers that this user dismissed suggestion on peer. help.getPromoData
// then lists it under dismissed_suggestions and omits it from pending_suggestions.
func (c *ConfigurationCore) HelpDismissSuggestion(in *mtproto.TLHelpDismissSuggestion) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Suggestions == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err := c.svcCtx.Suggestions.Dismiss(c.ctx, c.MD.UserId, in.GetPeer(), in.GetSuggestion()); err != nil {
		c.Logger.Errorf("help.dismissSuggestion - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
