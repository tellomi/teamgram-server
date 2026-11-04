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
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/helpdesk"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

// HelpGetAppUpdate
// help.getAppUpdate#522d5a7d source:string = help.AppUpdate;
// https://core.telegram.org/method/help.getAppUpdate
//
// Compares the signed-in client's app version with the published release.
// The release is latest.json: TELLOMI_APP_UPDATE_MANIFEST, a local synced
// file, or https://www.tellomi.cn/dl/android/latest.json. A non-empty
// TELLOMI_APP_UPDATE_VERSION replaces that manifest.
// help.noAppUpdate means the client is current or no release is published.
// iOS (session lang_pack "ios") always receives help.noAppUpdate: that client
// opens a blocking page whose button only launches App Store id 0.
// The version is the initConnection app_version when the session has it,
// otherwise a version embedded in source. A session that cannot be loaded
// keeps that comparison.
func (c *ConfigurationCore) HelpGetAppUpdate(in *mtproto.TLHelpGetAppUpdate) (*mtproto.Help_AppUpdate, error) {
	return helpdesk.AppUpdateForSession(c.updateClient(), in.GetSource(), helpdesk.LoadRelease()), nil
}

// updateClient reads lang_pack from the current RPC session, then the auths
// row. device_model, system_version, and app_version come from the same row.
// Known stays false when none of that is available.
func (c *ConfigurationCore) updateClient() helpdesk.UpdateClient {
	var client helpdesk.UpdateClient
	lang, langOK := c.sessionLangPack()
	client.LangPack = lang
	if strings.EqualFold(strings.TrimSpace(lang), "ios") {
		client.Known = true
		return client
	}
	if auth, ok := c.sessionAuthorization(); ok {
		client.Known = true
		client.DeviceModel = auth.GetDeviceModel()
		client.SystemVersion = auth.GetSystemVersion()
		client.AppVersion = auth.GetAppVersion()
	}
	if langOK {
		client.Known = true
	}
	return client
}

func (c *ConfigurationCore) sessionLangPack() (string, bool) {
	if c.MD != nil {
		if pack := strings.TrimSpace(c.MD.GetLangpack()); pack != "" {
			return pack, true
		}
	}
	if !c.canReadSession() {
		return "", false
	}
	packed, err := c.svcCtx.AuthSession.AuthsessionGetLangPack(c.ctx, &authsession.TLAuthsessionGetLangPack{
		AuthKeyId: c.MD.GetPermAuthKeyId(),
	})
	if err != nil || packed == nil {
		return "", false
	}
	return strings.TrimSpace(packed.GetV()), true
}

func (c *ConfigurationCore) sessionAuthorization() (*mtproto.Authorization, bool) {
	if !c.canReadSession() {
		return nil, false
	}
	auth, err := c.svcCtx.AuthSession.AuthsessionGetAuthorization(c.ctx, &authsession.TLAuthsessionGetAuthorization{
		AuthKeyId: c.MD.GetPermAuthKeyId(),
	})
	if err != nil || auth == nil {
		return nil, false
	}
	return auth, true
}

func (c *ConfigurationCore) canReadSession() bool {
	return c.MD != nil && c.MD.GetPermAuthKeyId() != 0 && c.svcCtx != nil && c.svcCtx.AuthSession != nil
}
