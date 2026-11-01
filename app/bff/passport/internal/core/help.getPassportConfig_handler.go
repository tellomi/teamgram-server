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
	"github.com/teamgram/teamgram-server/app/bff/passport/book"
)

// HelpGetPassportConfig
// help.getPassportConfig#c661ad08 hash:int = help.PassportConfig;
// https://core.telegram.org/method/help.getPassportConfig
//
// Returns the country-to-language map, or passportConfigNotModified when
// the client already has the same hash.
func (c *PassportCore) HelpGetPassportConfig(in *mtproto.TLHelpGetPassportConfig) (*mtproto.Help_PassportConfig, error) {
	if _, err := c.requireUser(); err != nil {
		c.Logger.Errorf("help.getPassportConfig - error: %v", err)
		return nil, err
	}
	if in.GetHash() != 0 && in.GetHash() == book.ConfigHash() {
		return mtproto.MakeTLHelpPassportConfigNotModified(&mtproto.Help_PassportConfig{}).To_Help_PassportConfig(), nil
	}
	return mtproto.MakeTLHelpPassportConfig(&mtproto.Help_PassportConfig{
		Hash: book.ConfigHash(),
		CountriesLangs: mtproto.MakeTLDataJSON(&mtproto.DataJSON{
			Data: book.CountriesLangs(),
		}).To_DataJSON(),
	}).To_Help_PassportConfig(), nil
}
