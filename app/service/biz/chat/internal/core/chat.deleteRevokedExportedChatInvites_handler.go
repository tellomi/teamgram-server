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

// ChatDeleteRevokedExportedChatInvites
// chat.deleteRevokedExportedChatInvites self_id:long chat_id:long admin_id:long = Bool;
func (c *ChatCore) ChatDeleteRevokedExportedChatInvites(in *chat.TLChatDeleteRevokedExportedChatInvites) (*mtproto.Bool, error) {
	chat2, err := c.svcCtx.Dao.GetMutableChat(c.ctx, in.ChatId)
	if err != nil {
		c.Logger.Errorf("chat.deleteRevokedExportedChatInvites - error: %v", err)
		return nil, err
	}
	me, _ := chat2.GetImmutableChatParticipant(in.SelfId)
	if me == nil || !me.IsChatMemberStateNormal() || !me.CanInviteUsers() {
		return nil, mtproto.ErrChatAdminInviteRequired
	}
	if in.AdminId != in.SelfId && !me.IsChatMemberCreator() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if _, err = c.svcCtx.Dao.ChatInvitesDAO.DeleteByRevoked(c.ctx, in.ChatId, in.AdminId); err != nil {
		c.Logger.Errorf("chat.deleteRevokedExportedChatInvites - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
