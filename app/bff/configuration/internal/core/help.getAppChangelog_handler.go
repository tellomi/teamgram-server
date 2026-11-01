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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/helpdesk"
)

// HelpGetAppChangelog
// help.getAppChangelog#9010ef6f prev_app_version:string = Updates;
// https://core.telegram.org/method/help.getAppChangelog
//
// Returns updateServiceNotification entries for changelog notes newer than
// prev_app_version. The client stores them on user 777000. An empty previous
// version, or a client that already has every note, returns updates with no
// entries.
func (c *ConfigurationCore) HelpGetAppChangelog(in *mtproto.TLHelpGetAppChangelog) (*mtproto.Updates, error) {
	return helpdesk.Changelog(in.GetPrevAppVersion(), helpdesk.LoadNotes(), int32(time.Now().Unix())), nil
}
