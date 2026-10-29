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
	"github.com/teamgram/teamgram-server/app/bff/tos/terms"
)

// HelpAcceptTermsOfService
// help.acceptTermsOfService#ee72f79a id:DataJSON = Bool;
//
// Persists acceptance for this user and terms id, then returns true.
func (c *TosCore) HelpAcceptTermsOfService(in *mtproto.TLHelpAcceptTermsOfService) (*mtproto.Bool, error) {
	raw := ""
	if in != nil && in.GetId() != nil {
		raw = in.GetId().GetData()
	}
	if err := c.terms().Accept(c.ctx, c.userID(), raw); err != nil {
		c.Logger.Errorf("help.acceptTermsOfService: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.BoolTrue, nil
}

func (c *TosCore) terms() *terms.Service {
	if c != nil && c.svcCtx != nil && c.svcCtx.Terms != nil {
		return c.svcCtx.Terms
	}
	return terms.Current()
}

func (c *TosCore) userID() int64 {
	if c == nil || c.MD == nil {
		return 0
	}
	return c.MD.GetUserId()
}
