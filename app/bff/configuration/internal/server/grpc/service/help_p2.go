package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/core"
)

// HelpGetTimezonesList
// help.getTimezonesList#49b30240 hash:int = help.TimezonesList;
func (s *Service) HelpGetTimezonesList(ctx context.Context, request *mtproto.TLHelpGetTimezonesList) (*mtproto.Help_TimezonesList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("help.getTimezonesList - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.HelpGetTimezonesList(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("help.getTimezonesList - reply: {%s}", r)
	return r, err
}

// HelpGetUserInfo
// help.getUserInfo#38a08d3 user_id:InputUser = help.UserInfo;
func (s *Service) HelpGetUserInfo(ctx context.Context, request *mtproto.TLHelpGetUserInfo) (*mtproto.Help_UserInfo, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("help.getUserInfo - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.HelpGetUserInfo(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("help.getUserInfo - reply: {%s}", r)
	return r, err
}

// HelpEditUserInfo
// help.editUserInfo#66b91b70 user_id:InputUser message:string entities:Vector<MessageEntity> = help.UserInfo;
func (s *Service) HelpEditUserInfo(ctx context.Context, request *mtproto.TLHelpEditUserInfo) (*mtproto.Help_UserInfo, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("help.editUserInfo - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.HelpEditUserInfo(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("help.editUserInfo - reply: {%s}", r)
	return r, err
}

var (
	_ mtproto.RPCTimezonesServer = (*Service)(nil)
	_ mtproto.RPCTsfServer       = (*Service)(nil)
)
