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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/pkg/invite"
)

// ChatEditExportedChatInvite
// chat.editExportedChatInvite flags:# self_id:long chat_id:long revoked:flags.2?true link:string expire_date:flags.0?int usage_limit:flags.1?int request_needed:flags.3?Bool title:flags.4?string = ExportedChatInvite;
func (c *ChatCore) ChatEditExportedChatInvite(in *chat.TLChatEditExportedChatInvite) (*chat.Vector_ExportedChatInvite, error) {
	var (
		hash        = chat.GetInviteHashByLink(in.Link)
		chatInvites = make([]*mtproto.ExportedChatInvite, 0, 2)
	)

	chatInviteDO, err := c.svcCtx.Dao.ChatInvitesDAO.SelectByLink(c.ctx, hash)
	if err != nil {
		c.Logger.Errorf("chat.editExportedChatInvite - error: %v", err)
		return nil, err
	} else if chatInviteDO == nil || chatInviteDO.ChatId != in.ChatId {
		return nil, mtproto.ErrInviteHashInvalid
	}
	chat2, err := c.svcCtx.Dao.GetMutableChat(c.ctx, in.ChatId)
	if err != nil {
		c.Logger.Errorf("chat.editExportedChatInvite - error: %v", err)
		return nil, err
	}
	me, _ := chat2.GetImmutableChatParticipant(in.SelfId)
	if me == nil || !me.IsChatMemberStateNormal() || !me.CanInviteUsers() {
		return nil, mtproto.ErrChatAdminInviteRequired
	}
	if chatInviteDO.AdminId != in.SelfId && !me.IsChatMemberCreator() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if chatInviteDO.Revoked {
		return nil, mtproto.ErrInviteHashExpired
	}

	if in.Revoked {
		if _, err := c.svcCtx.Dao.ChatInvitesDAO.Update(c.ctx, map[string]interface{}{
			"revoked": true,
		}, in.ChatId, hash); err != nil {
			c.Logger.Errorf("chat.editExportedChatInvite - error: %v", err)
			return nil, err
		}
		chatInviteDO.Revoked = true
		chatInvites = append(chatInvites, c.svcCtx.Dao.MakeChatInviteExported(c.ctx, chatInviteDO))

		if chatInviteDO.Permanent {
			fresh := &dataobject.ChatInvitesDO{
				ChatId:    in.ChatId,
				AdminId:   chatInviteDO.AdminId,
				Link:      chat.GenChatInviteHash(),
				Permanent: true,
				Date2:     time.Now().Unix(),
			}
			if _, _, err := c.svcCtx.Dao.ChatInvitesDAO.Insert(c.ctx, fresh); err != nil {
				c.Logger.Errorf("chat.editExportedChatInvite - error: %v", err)
				return nil, err
			}
			chatInvites = append(chatInvites, c.svcCtx.Dao.MakeChatInviteExported(c.ctx, fresh))
			_, _ = c.svcCtx.Dao.ChatParticipantsDAO.UpdateLink(c.ctx, fresh.Link, in.ChatId, fresh.AdminId)
		}
	} else {
		if chatInviteDO.Permanent && (in.GetExpireDate() != nil || in.GetUsageLimit() != nil || in.GetRequestNeeded() != nil || in.GetTitle() != nil) {
			return nil, mtproto.ErrChatInvitePermanent
		}
		cMap := map[string]interface{}{}
		now := time.Now().Unix()

		if in.GetExpireDate() != nil {
			exp := int64(in.GetExpireDate().GetValue())
			if exp != 0 && exp <= now {
				return nil, mtproto.ErrExpireDateInvalid
			}
			cMap["expire_date"] = exp
			chatInviteDO.ExpireDate = exp
		}
		if in.GetUsageLimit() != nil {
			if in.GetUsageLimit().GetValue() < 0 {
				return nil, mtproto.ErrUsageLimitInvalid
			}
			cMap["usage_limit"] = in.GetUsageLimit().GetValue()
			chatInviteDO.UsageLimit = in.GetUsageLimit().GetValue()
		}
		if in.GetRequestNeeded() != nil {
			cMap["request_needed"] = mtproto.FromBool(in.GetRequestNeeded())
			chatInviteDO.RequestNeeded = mtproto.FromBool(in.GetRequestNeeded())
		}
		if in.GetTitle() != nil {
			if !invite.TitleOK(in.GetTitle().GetValue()) {
				return nil, mtproto.ErrTitleInvalid
			}
			cMap["title"] = in.GetTitle().GetValue()
			chatInviteDO.Title = in.GetTitle().GetValue()
		}

		if len(cMap) > 0 {
			if _, err := c.svcCtx.Dao.ChatInvitesDAO.Update(c.ctx, cMap, in.ChatId, hash); err != nil {
				c.Logger.Errorf("chat.editExportedChatInvite - error: %v", err)
				return nil, err
			}
		}
		chatInvites = append(chatInvites, c.svcCtx.Dao.MakeChatInviteExported(c.ctx, chatInviteDO))
	}

	return &chat.Vector_ExportedChatInvite{
		Datas: chatInvites,
	}, nil
}
