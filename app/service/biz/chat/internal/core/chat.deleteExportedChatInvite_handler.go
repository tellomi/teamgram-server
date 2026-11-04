/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// ChatDeleteExportedChatInvite
// chat.deleteExportedChatInvite self_id:long chat_id:long link:string = Bool;
func (c *ChatCore) ChatDeleteExportedChatInvite(in *chat.TLChatDeleteExportedChatInvite) (*mtproto.Bool, error) {
	link := chat.GetInviteHashByLink(in.Link)
	row, err := c.svcCtx.Dao.ChatInvitesDAO.SelectByLink(c.ctx, link)
	if err != nil {
		c.Logger.Errorf("chat.deleteExportedChatInvite - error: %v", err)
		return nil, err
	}
	if row == nil || row.ChatId != in.ChatId {
		return nil, mtproto.ErrInviteHashInvalid
	}
	chat2, err := c.svcCtx.Dao.GetMutableChat(c.ctx, in.ChatId)
	if err != nil {
		c.Logger.Errorf("chat.deleteExportedChatInvite - error: %v", err)
		return nil, err
	}
	me, _ := chat2.GetImmutableChatParticipant(in.SelfId)
	if me == nil || !me.IsChatMemberStateNormal() || !me.CanInviteUsers() {
		return nil, mtproto.ErrChatAdminInviteRequired
	}
	if row.AdminId != in.SelfId && !me.IsChatMemberCreator() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if _, err = c.svcCtx.Dao.ChatInvitesDAO.DeleteByLink(c.ctx, in.ChatId, link); err != nil {
		c.Logger.Errorf("chat.deleteExportedChatInvite - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
