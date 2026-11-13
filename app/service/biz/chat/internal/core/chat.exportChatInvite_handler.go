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
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/pkg/invite"
)

// ChatExportChatInvite
// chat.exportChatInvite flags:# chat_id:long admin_id:long legacy_revoke_permanent:flags.2?true request_needed:flags.3?true expire_date:flags.0?int usage_limit:flags.1?int title:flags.4?string = ExportedChatInvite;
func (c *ChatCore) ChatExportChatInvite(in *chat.TLChatExportChatInvite) (*mtproto.ExportedChatInvite, error) {
	chat2, err := c.svcCtx.Dao.GetMutableChat(c.ctx, in.ChatId)
	if err != nil {
		c.Logger.Errorf("chat.exportChatInvite - error: %v", err)
		return nil, err
	}
	if chat2.Deactivated() && chat2.GetChat().GetMigratedTo() != nil {
		return nil, mtproto.ErrMigratedToChannel
	}
	me, _ := chat2.GetImmutableChatParticipant(in.AdminId)
	if me == nil || !me.IsChatMemberStateNormal() || !me.CanInviteUsers() {
		return nil, mtproto.ErrChatAdminInviteRequired
	}

	now := time.Now().Unix()
	expire := int64(in.GetExpireDate().GetValue())
	title := in.GetTitle().GetValue()
	exp := invite.Export{
		Legacy:        in.LegacyRevokePermanent,
		RequestNeeded: in.RequestNeeded,
		Expire:        expire,
		Usage:         in.GetUsageLimit().GetValue(),
		UsageSet:      in.GetUsageLimit() != nil,
		Title:         title,
		TitleSet:      in.GetTitle() != nil,
		Now:           now,
	}
	if err := exp.Validate(); err != nil {
		return nil, err
	}
	usage := exp.StoredUsage()

	// The share sheet sends legacy_revoke_permanent when the group has no
	// primary link. That revokes the previous permanent link and writes a new one.
	// The "new link" screen leaves the flag unset and sends title, approval,
	// expiry, and usage instead.
	if in.LegacyRevokePermanent {
		rows, err := c.svcCtx.Dao.ChatInvitesDAO.SelectListByAdminId(c.ctx, in.ChatId, in.AdminId)
		if err != nil {
			c.Logger.Errorf("chat.exportChatInvite - error: %v", err)
			return nil, err
		}
		for i := range rows {
			if rows[i].Permanent && !rows[i].Revoked {
				if _, err := c.svcCtx.Dao.ChatInvitesDAO.Update(c.ctx, map[string]interface{}{
					"revoked": true,
				}, in.ChatId, rows[i].Link); err != nil {
					c.Logger.Errorf("chat.exportChatInvite - error: %v", err)
					return nil, err
				}
			}
		}
	}

	row := &dataobject.ChatInvitesDO{
		ChatId:        in.ChatId,
		AdminId:       in.AdminId,
		Permanent:     in.LegacyRevokePermanent,
		Revoked:       false,
		RequestNeeded: in.RequestNeeded && !in.LegacyRevokePermanent,
		StartDate:     0,
		ExpireDate:    expire,
		UsageLimit:    usage,
		Usage2:        0,
		Requested:     0,
		Title:         title,
		Date2:         now,
	}
	if in.LegacyRevokePermanent {
		row.ExpireDate = 0
		row.UsageLimit = 0
		row.Title = ""
		row.RequestNeeded = false
	}
	var lastErr error
	for i := 0; i < 5; i++ {
		row.Link = chat.GenChatInviteHash()
		_, _, lastErr = c.svcCtx.Dao.ChatInvitesDAO.Insert(c.ctx, row)
		if lastErr == nil {
			break
		}
		if !strings.Contains(strings.ToLower(lastErr.Error()), "duplicate") {
			break
		}
	}
	if lastErr != nil {
		c.Logger.Errorf("chat.exportChatInvite - error: %v", lastErr)
		return nil, lastErr
	}
	if row.Permanent {
		_, _ = c.svcCtx.Dao.ChatParticipantsDAO.UpdateLink(c.ctx, row.Link, in.ChatId, in.AdminId)
	}
	return c.svcCtx.Dao.MakeChatInviteExported(c.ctx, row), nil
}
