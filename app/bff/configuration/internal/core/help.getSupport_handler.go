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
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// HelpGetSupport
// help.getSupport#9cdf08cd = help.Support;
// https://core.telegram.org/method/help.getSupport
//
// Returns the support account used by "ask a question". The account is
// TELLOMI_SUPPORT_USER_ID when set, otherwise the user named "support".
// A missing account is 403 USER_INVALID so the client does not open a dead chat.
func (c *ConfigurationCore) HelpGetSupport(in *mtproto.TLHelpGetSupport) (*mtproto.Help_Support, error) {
	_ = in
	user, err := c.supportUser()
	if err != nil {
		c.Logger.Errorf("help.getSupport - error: %v", err)
		return nil, err
	}
	return mtproto.MakeTLHelpSupport(&mtproto.Help_Support{
		PhoneNumber: user.GetPhone().GetValue(),
		User:        user,
	}).To_Help_Support(), nil
}

// HelpGetSupportName
// help.getSupportName#d360e72c = help.SupportName;
// https://core.telegram.org/method/help.getSupportName
func (c *ConfigurationCore) HelpGetSupportName(in *mtproto.TLHelpGetSupportName) (*mtproto.Help_SupportName, error) {
	_ = in
	user, err := c.supportUser()
	if err != nil {
		c.Logger.Errorf("help.getSupportName - error: %v", err)
		return nil, err
	}
	lang := ""
	if c.MD != nil {
		lang = c.MD.GetLangCode()
	}
	return mtproto.MakeTLHelpSupportName(&mtproto.Help_SupportName{
		Name: supportLabel(lang, user.GetFirstName().GetValue()),
	}).To_Help_SupportName(), nil
}

func (c *ConfigurationCore) supportUser() (*mtproto.User, error) {
	if c.svcCtx == nil || c.svcCtx.UserClient == nil {
		return nil, mtproto.ErrForbiddenUserInvalid
	}
	id, err := c.supportUserID()
	if err != nil || id == 0 {
		return nil, mtproto.ErrForbiddenUserInvalid
	}
	self := id
	if c.MD != nil && c.MD.UserId != 0 {
		self = c.MD.UserId
	}
	users, err := c.svcCtx.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{self, id},
	})
	if err != nil || users == nil {
		return nil, mtproto.ErrForbiddenUserInvalid
	}
	list := users.GetUserListByIdList(self, id)
	if len(list) == 0 || list[0] == nil || list[0].GetDeleted() {
		return nil, mtproto.ErrForbiddenUserInvalid
	}
	return list[0], nil
}

func (c *ConfigurationCore) supportUserID() (int64, error) {
	if raw := strings.TrimSpace(os.Getenv("TELLOMI_SUPPORT_USER_ID")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return 0, mtproto.ErrForbiddenUserInvalid
		}
		return id, nil
	}
	found, err := c.svcCtx.UserClient.UserGetListByUsernameList(c.ctx, &userpb.TLUserGetListByUsernameList{
		Names: []string{"support"},
	})
	if err != nil || found == nil {
		return 0, err
	}
	for _, row := range found.GetDatas() {
		if row.GetPeer().GetUserId() != 0 {
			return row.GetPeer().GetUserId(), nil
		}
	}
	return 0, nil
}
