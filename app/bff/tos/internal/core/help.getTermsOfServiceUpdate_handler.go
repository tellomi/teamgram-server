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
	"github.com/teamgram/teamgram-server/pkg/brand"
)

// HelpGetTermsOfServiceUpdate
// help.getTermsOfServiceUpdate#2ca51fd1 = help.TermsOfServiceUpdate;
func (c *TosCore) HelpGetTermsOfServiceUpdate(in *mtproto.TLHelpGetTermsOfServiceUpdate) (*mtproto.Help_TermsOfServiceUpdate, error) {
	text := brand.TermsURL + "\n" + brand.PrivacyURL
	termsLen := int32(len(brand.TermsURL))
	privacyLen := int32(len(brand.PrivacyURL))
	return mtproto.MakeTLHelpTermsOfServiceUpdate(&mtproto.Help_TermsOfServiceUpdate{
		Expires: int32(time.Now().Unix() + 3600),
		TermsOfService: mtproto.MakeTLHelpTermsOfService(&mtproto.Help_TermsOfService{
			Popup: false,
			Id:    mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: `{"id":"tellomi-tos-1"}`}).To_DataJSON(),
			Text:  text,
			Entities: []*mtproto.MessageEntity{
				mtproto.MakeTLMessageEntityUrl(&mtproto.MessageEntity{Offset: 0, Length: termsLen}).To_MessageEntity(),
				mtproto.MakeTLMessageEntityUrl(&mtproto.MessageEntity{Offset: termsLen + 1, Length: privacyLen}).To_MessageEntity(),
			},
		}).To_Help_TermsOfService(),
	}).To_Help_TermsOfServiceUpdate(), nil
}
