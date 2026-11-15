package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/userinfo"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// HelpGetUserInfo
// help.getUserInfo#38a08d3 user_id:InputUser = help.UserInfo;
// https://core.telegram.org/method/help.getUserInfo
//
// Internal TSF method. A caller who is not support gets 403 USER_INVALID,
// including the user the note is about.
func (c *ConfigurationCore) HelpGetUserInfo(in *mtproto.TLHelpGetUserInfo) (*mtproto.Help_UserInfo, error) {
	if err := c.requireSupport(); err != nil {
		return nil, err
	}
	id, err := c.noteUser(in.GetUserId())
	if err != nil {
		return nil, err
	}
	return c.notes().Get(c.ctx, id)
}

// HelpEditUserInfo
// help.editUserInfo#66b91b70 user_id:InputUser message:string entities:Vector<MessageEntity> = help.UserInfo;
// https://core.telegram.org/method/help.editUserInfo
//
// Internal TSF method. Only a support account may write the note.
func (c *ConfigurationCore) HelpEditUserInfo(in *mtproto.TLHelpEditUserInfo) (*mtproto.Help_UserInfo, error) {
	if err := c.requireSupport(); err != nil {
		return nil, err
	}
	id, err := c.noteUser(in.GetUserId())
	if err != nil {
		return nil, err
	}
	return c.notes().Edit(c.ctx, c.selfID(), id, in.GetMessage(), in.GetEntities())
}

// requireSupport allows the configured support account and any user whose
// users.support flag is set. Anyone else gets 403 USER_INVALID.
func (c *ConfigurationCore) requireSupport() error {
	self := c.selfID()
	if self == 0 {
		return mtproto.ErrAuthKeyUnregistered
	}
	ok, err := c.supportCaller(self)
	if err != nil {
		return err
	}
	if !ok {
		return mtproto.ErrForbiddenUserInvalid
	}
	return nil
}

func (c *ConfigurationCore) supportCaller(id int64) (bool, error) {
	if c.svcCtx != nil && c.svcCtx.Support != nil {
		return c.svcCtx.Support(c.ctx, id)
	}
	if c.svcCtx == nil || c.svcCtx.UserClient == nil {
		return false, nil
	}
	if sid, err := c.supportUserID(); err == nil && sid != 0 && sid == id {
		return true, nil
	}
	users, err := c.svcCtx.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{id},
	})
	if err != nil || users == nil {
		if err != nil {
			return false, err
		}
		return false, nil
	}
	list := users.GetUserListByIdList(id, id)
	if len(list) == 0 || list[0] == nil {
		return false, nil
	}
	return list[0].GetSupport(), nil
}

func (c *ConfigurationCore) notes() *userinfo.Store {
	if c.svcCtx != nil && c.svcCtx.UserInfo != nil {
		return c.svcCtx.UserInfo
	}
	return userinfo.NewMemory()
}

func (c *ConfigurationCore) selfID() int64 {
	if c.MD == nil {
		return 0
	}
	return c.MD.GetUserId()
}

func (c *ConfigurationCore) noteUser(in *mtproto.InputUser) (int64, error) {
	if in == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	id := mtproto.ToUserIdByInput(c.selfID(), in)
	if id <= 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	return id, nil
}
