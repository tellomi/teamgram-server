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
	"sort"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

// ChatGetExportedChatInvites
// chat.getExportedChatInvites flags:# chat_id:long admin_id:long revoked:flags.3?true offset_date:flags.2?int offset_link:flags.2?string limit:int = Vector<ExportedChatInvite>;
func (c *ChatCore) ChatGetExportedChatInvites(in *chat.TLChatGetExportedChatInvites) (*chat.Vector_ExportedChatInvite, error) {
	var rInvites []*mtproto.ExportedChatInvite

	_, err := c.svcCtx.Dao.ChatInvitesDAO.SelectListByAdminIdWithCB(
		c.ctx,
		in.ChatId,
		in.AdminId,
		func(sz, i int, v *dataobject.ChatInvitesDO) {
			if v.Revoked == in.Revoked {
				rInvites = append(rInvites, c.svcCtx.Dao.MakeChatInviteExported(c.ctx, v))
			}
		})
	if err != nil {
		c.Logger.Errorf("chat.getExportedChatInvites - error: %v", err)
		return nil, err
	}
	if rInvites == nil {
		rInvites = []*mtproto.ExportedChatInvite{}
	}
	sort.Slice(rInvites, func(i, j int) bool {
		if rInvites[i].GetDate() == rInvites[j].GetDate() {
			return rInvites[i].GetLink() > rInvites[j].GetLink()
		}
		return rInvites[i].GetDate() > rInvites[j].GetDate()
	})

	return &chat.Vector_ExportedChatInvite{
		Datas: rInvites,
	}, nil
}
