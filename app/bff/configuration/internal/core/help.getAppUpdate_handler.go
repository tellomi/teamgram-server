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
	"github.com/teamgram/teamgram-server/app/bff/helpdesk"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

// HelpGetAppUpdate
// help.getAppUpdate#522d5a7d source:string = help.AppUpdate;
// https://core.telegram.org/method/help.getAppUpdate
//
// Compares the signed-in client's app version with the published release.
// help.noAppUpdate means the client is current or no release is published.
// The version is the initConnection app_version when the session has it,
// otherwise a version embedded in source.
func (c *ConfigurationCore) HelpGetAppUpdate(in *mtproto.TLHelpGetAppUpdate) (*mtproto.Help_AppUpdate, error) {
	return helpdesk.AppUpdate(c.clientVersion(in.GetSource()), in.GetSource(), helpdesk.LoadRelease()), nil
}

func (c *ConfigurationCore) clientVersion(source string) string {
	if v := helpdesk.VersionInSource(source); v != "" {
		return v
	}
	if c.MD == nil || c.MD.GetPermAuthKeyId() == 0 || c.svcCtx == nil || c.svcCtx.AuthSession == nil {
		return ""
	}
	auth, err := c.svcCtx.AuthSession.AuthsessionGetAuthorization(c.ctx, &authsession.TLAuthsessionGetAuthorization{
		AuthKeyId: c.MD.GetPermAuthKeyId(),
	})
	if err != nil || auth == nil {
		return ""
	}
	return auth.GetAppVersion()
}
