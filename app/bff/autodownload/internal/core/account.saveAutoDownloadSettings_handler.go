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

// AccountSaveAutoDownloadSettings
// account.saveAutoDownloadSettings#76f36233 flags:# low:flags.0?true high:flags.1?true settings:AutoDownloadSettings = Bool;
//
// https://core.telegram.org/method/account.saveAutoDownloadSettings
//
// low saves the low preset, high saves the high preset, and neither saves
// medium. account.getAutoDownloadSettings returns the saved preset.
func (c *AutoDownloadCore) AccountSaveAutoDownloadSettings(in *mtproto.TLAccountSaveAutoDownloadSettings) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err := presets.Shared().Save(c.ctx, c.MD.UserId, in.GetLow(), in.GetHigh(), in.GetSettings()); err != nil {
		c.Logger.Errorf("account.saveAutoDownloadSettings - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
