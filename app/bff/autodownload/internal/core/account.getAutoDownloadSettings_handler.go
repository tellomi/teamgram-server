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
	"github.com/teamgram/teamgram-server/app/bff/autodownload/presets"
)

// AccountGetAutoDownloadSettings
// account.getAutoDownloadSettings#56da0b3f = account.AutoDownloadSettings;
//
// https://core.telegram.org/method/account.getAutoDownloadSettings
//
// Returns the low, medium, and high presets. A preset the user has saved
// with account.saveAutoDownloadSettings is returned as saved. The others
// stay at the fresh-account defaults.
func (c *AutoDownloadCore) AccountGetAutoDownloadSettings(in *mtproto.TLAccountGetAutoDownloadSettings) (*mtproto.Account_AutoDownloadSettings, error) {
	userID := int64(0)
	if c.MD != nil {
		userID = c.MD.UserId
	}
	got, err := presets.Shared().Get(c.ctx, userID)
	if err != nil {
		c.Logger.Errorf("account.getAutoDownloadSettings - error: %v", err)
		return nil, err
	}
	return got, nil
}
