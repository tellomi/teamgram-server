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
	"os"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/teamgram/teamgram-server/pkg/langpack"

	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/proto"
)

const (
	configFile = "./config.json"
	// date = 1509066502,    2017/10/27 09:08:22
	// expires = 1509070295, 2017/10/27 10:11:35
	expiresTimeout = 3600 // 超时时间设置为3600秒

	// support user: @benqi
	// SUPPORT_USER_ID = 2
)

var config mtproto.TLConfig

func init() {
	configData, err := os.ReadFile(configFile)
	if err != nil {
		panic(err)
		return
	}

	err = jsonx.Unmarshal(configData, &config)
	if err != nil {
		panic(err)
		return
	}
}

// HelpGetConfig
// help.getConfig#c4f9186b = Config;
func (c *ConfigurationCore) HelpGetConfig(in *mtproto.TLHelpGetConfig) (*mtproto.Config, error) {
	_ = in

	rValue, _ := proto.Clone(&config).(*mtproto.TLConfig)
	now := int32(time.Now().Unix())
	rValue.SetDate(now)
	rValue.SetExpires(now + expiresTimeout)
	rValue.SetMeUrlPrefix(brand.LinkPrefix)

	var langCode, systemLang, pack string
	if c.MD != nil {
		langCode = c.MD.GetLangCode()
		systemLang = c.MD.GetSystemLangCode()
		pack = c.MD.GetLangpack()
	}
	// iOS reads suggested_lang_code from this config. Android shows its language
	// chooser from the same field. Map the device/app language onto zh-hans or zh-hant.
	langpack.ApplyConfigLanguage(rValue, langCode, systemLang, pack)
	// Double-tap sends this emoji. It has to be one of messages.getAvailableReactions.
	// messages.setDefaultReaction replaces the account default.
	reaction := mtproto.MakeTLReactionEmoji(&mtproto.Reaction{
		Emoticon: catalog.DefaultReactionEmoji,
	}).To_Reaction()
	if c.MD != nil {
		if chosen := chatbox.Shared().DefaultReaction(c.ctx, c.MD.GetUserId()); chosen != nil {
			reaction = chosen
		}
	}
	rValue.SetReactionsDefault(reaction)

	return rValue.To_Config(), nil
}
