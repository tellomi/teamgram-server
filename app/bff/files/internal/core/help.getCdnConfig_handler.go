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
)

// HelpGetCdnConfig
// help.getCdnConfig#52029342 = CdnConfig;
// https://core.telegram.org/method/help.getCdnConfig
//
// Returns the RSA public keys for secondary CDN datacenters. An empty vector
// means this server does not publish a CDN, so clients download from the main DC.
func (c *FilesCore) HelpGetCdnConfig(in *mtproto.TLHelpGetCdnConfig) (*mtproto.CdnConfig, error) {
	_ = in
	keys := helpdesk.PublicKeys()
	out := make([]*mtproto.CdnPublicKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, mtproto.MakeTLCdnPublicKey(&mtproto.CdnPublicKey{
			DcId:      key.DC,
			PublicKey: key.PEM,
		}).To_CdnPublicKey())
	}
	return mtproto.MakeTLCdnConfig(&mtproto.CdnConfig{PublicKeys: out}).To_CdnConfig(), nil
}
