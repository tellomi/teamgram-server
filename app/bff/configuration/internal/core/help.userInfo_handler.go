package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/userinfo"
)

// HelpGetUserInfo
// help.getUserInfo#38a08d3 user_id:InputUser = help.UserInfo;
func (c *ConfigurationCore) HelpGetUserInfo(in *mtproto.TLHelpGetUserInfo) (*mtproto.Help_UserInfo, error) {
	id, err := c.noteUser(in.GetUserId())
	if err != nil {
		return nil, err
	}
	return c.notes().Get(c.ctx, id)
}

// HelpEditUserInfo
// help.editUserInfo#66b91b70 user_id:InputUser message:string entities:Vector<MessageEntity> = help.UserInfo;
func (c *ConfigurationCore) HelpEditUserInfo(in *mtproto.TLHelpEditUserInfo) (*mtproto.Help_UserInfo, error) {
	id, err := c.noteUser(in.GetUserId())
	if err != nil {
		return nil, err
	}
	return c.notes().Edit(c.ctx, c.selfID(), id, in.GetMessage(), in.GetEntities())
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
