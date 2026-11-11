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
	"context"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/ttlmedia"
	"github.com/zeromicro/go-zero/core/jsonx"
)

// MsgReadMessageContents
// msg.readMessageContents user_id:long auth_key_id:long peer_type:int peer_id:long id:Vector<ContentMessage> = messages.AffectedMessage;
func (c *MsgCore) MsgReadMessageContents(in *msg.TLMsgReadMessageContents) (*mtproto.Messages_AffectedMessages, error) {
	var (
		pts, ptsCount int32
	)

	affected, _ := c.readMentionedMessageContents(in)
	ptsCount += affected
	affected, _ = c.readMediaUnreadMessageContents(in)
	ptsCount += affected
	affected, _ = c.readReactionUnreadMessageContents(in)
	ptsCount += affected

	if ptsCount > 0 {
		pts = c.svcCtx.Dao.IDGenClient2.NextNPtsId(c.ctx, in.UserId, int(ptsCount))
		// The reader's other devices sync through getDifference. The live
		// push is SyncUpdatesNotMe in the bff; this row is what an offline
		// session replays. date is the open time Android uses for the countdown.
		ids := make([]int32, 0, len(in.Id))
		for _, m := range in.Id {
			ids = append(ids, m.Id)
		}
		upd := mtproto.MakeUpdateReadMessagesContents(ids, pts, ptsCount, int32(time.Now().Unix()))
		if _, err := c.svcCtx.Dao.AddToPtsQueueE(c.ctx, in.UserId, pts, ptsCount, upd); err != nil {
			c.Logger.Errorf("readMessageContents - persist reader pts: %v", err)
		}
	} else {
		ptsCount = 0
		pts = c.svcCtx.Dao.IDGenClient2.CurrentPtsId(c.ctx, in.UserId)
	}

	return mtproto.MakeTLMessagesAffectedMessages(&mtproto.Messages_AffectedMessages{
		Pts:      pts,
		PtsCount: ptsCount,
	}).To_Messages_AffectedMessages(), nil
}

func (c *MsgCore) readMentionedMessageContents(in *msg.TLMsgReadMessageContents) (int32, error) {
	var (
		ptsCount int32 = 0
	)

	switch in.PeerType {
	case mtproto.PEER_USER:
		return 0, nil
	case mtproto.PEER_CHAT:
		for _, m := range in.Id {
			if m.Mentioned {
				ptsCount++
			}
		}
		if ptsCount > 0 {
			sz := c.svcCtx.Dao.CommonDAO.CalcSize(
				c.ctx,
				c.svcCtx.Dao.MessagesDAO.CalcTableName(in.UserId),
				map[string]interface{}{
					"user_id":   in.UserId,
					"peer_type": mtproto.PEER_CHAT,
					"peer_id":   in.PeerId,
					"mentioned": 1,
					"deleted":   0,
				})
			for _, m := range in.Id {
				if m.Mentioned {
					c.svcCtx.Dao.MessagesDAO.UpdateMentionedAndMediaUnread(c.ctx, in.UserId, m.Id)
					//UpdateMentioned()
				}
			}

			sz = sz - int(ptsCount)
			if sz < 0 {
				sz = 0
			}

			c.svcCtx.Dao.ExecInvalidate(
				c.ctx,
				func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
					_, err2 := c.svcCtx.Dao.DialogsDAO.UpdateCustomMap(
						c.ctx,
						map[string]interface{}{
							"unread_mentions_count": sz,
						},
						in.UserId,
						mtproto.PEER_CHAT,
						in.PeerId)

					return 0, 0, err2
				},
				dialog.GetDialogCacheKey(in.UserId, mtproto.MakePeerDialogId(mtproto.PEER_CHAT, in.PeerId)))
		}

		return ptsCount, nil
	default:
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("DeleteMessages - error: %v", err)

		return 0, err
	}
}

func (c *MsgCore) readMediaUnreadMessageContents(in *msg.TLMsgReadMessageContents) (int32, error) {
	var (
		ptsCount int32 = 0
	)

	switch in.PeerType {
	case mtproto.PEER_USER:
		// id := make([]*inbox.InboxMessageId, 0, len(in.Id))
		for _, m := range in.Id {
			if m.MediaUnread {
				ptsCount++
				c.svcCtx.Dao.MessagesDAO.UpdateMediaUnread(c.ctx, in.UserId, m.Id)
				due, dialogID := c.noteTTLOpen(in.UserId, m.Id)
				if in.UserId != in.PeerId {
					c.svcCtx.Dao.InboxClient.InboxReadMediaUnreadToInboxV2(
						c.ctx, &inbox.TLInboxReadMediaUnreadToInboxV2{
							UserId:          in.PeerId,
							PeerType:        mtproto.PEER_USER,
							PeerId:          in.UserId,
							DialogMessageId: m.DialogMessageId,
						})
				}
				if due {
					if st := ttlmedia.Default(); st != nil {
						if err := st.Strip(c.ctx, dialogID); err != nil {
							c.Logger.Errorf("ttl media strip dialog %d: %v", dialogID, err)
						}
					}
				}
			}
		}

		return ptsCount, nil
	case mtproto.PEER_CHAT:
		// TODO: update sender
		for _, m := range in.Id {
			if m.MediaUnread {
				ptsCount++
				c.svcCtx.Dao.MessagesDAO.UpdateMediaUnread(c.ctx, in.UserId, m.Id)
				due, dialogID := c.noteTTLOpen(in.UserId, m.Id)
				c.svcCtx.Dao.InboxClient.InboxReadMediaUnreadToInboxV2(
					c.ctx, &inbox.TLInboxReadMediaUnreadToInboxV2{
						UserId:          m.SendUserId,
						PeerType:        mtproto.PEER_CHAT,
						PeerId:          in.PeerId,
						DialogMessageId: m.DialogMessageId,
					})
				if due {
					if st := ttlmedia.Default(); st != nil {
						if err := st.Strip(c.ctx, dialogID); err != nil {
							c.Logger.Errorf("ttl media strip dialog %d: %v", dialogID, err)
						}
					}
				}
			}
		}

		return ptsCount, nil
	default:
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("DeleteMessages - error: %v", err)

		return 0, err
	}
}

func (c *MsgCore) readReactionUnreadMessageContents(in *msg.TLMsgReadMessageContents) (int32, error) {
	var (
		unreadReactionsCount int32
	)

	for _, m := range in.Id {
		if m.Reaction {
			unreadReactionsCount++
		}
	}
	for _, m := range in.Id {
		if m.Reaction {
			if c.svcCtx.MsgPlugin != nil {
				c.svcCtx.MsgPlugin.ReadReactionUnreadMessage(c.ctx, in.UserId, m.Id)
			}
		}
	}

	if unreadReactionsCount > 0 {
		c.svcCtx.Dao.ExecInvalidate(
			c.ctx,
			func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
				_, err2 := c.svcCtx.Dao.DialogsDAO.UpdateUnreadCount(
					c.ctx,
					0,
					0,
					-unreadReactionsCount,
					in.UserId,
					in.PeerType,
					in.PeerId)

				return 0, 0, err2
			},
			dialog.GetDialogCacheKey(in.UserId, mtproto.MakePeerDialogId(in.PeerType, in.PeerId)))
	}
	return 0, nil
}

// noteTTLOpen starts the self-destruct clock when the recipient opens ttl
// media. The sender's own outgoing copy is not an open: official clients
// start the timer only on the incoming side. A saved-message chat is the
// exception, because the reader is the only participant.
func (c *MsgCore) noteTTLOpen(userID int64, boxID int32) (bool, int64) {
	st := ttlmedia.Default()
	if st == nil {
		return false, 0
	}
	do, err := c.svcCtx.Dao.MessagesDAO.SelectByMessageId(c.ctx, userID, boxID)
	if err != nil || do == nil {
		return false, 0
	}
	selfChat := do.PeerType == mtproto.PEER_SELF || (do.PeerType == mtproto.PEER_USER && do.PeerId == userID)
	if do.SenderUserId == userID && !selfChat {
		return false, 0
	}
	var msg mtproto.Message
	if jsonx.UnmarshalFromString(do.MessageData, &msg) != nil {
		return false, 0
	}
	due, err := st.Opened(c.ctx, do.DialogMessageId, &msg, int32(time.Now().Unix()))
	if err != nil {
		c.Logger.Errorf("ttl media open dialog %d: %v", do.DialogMessageId, err)
		return false, 0
	}
	return due, do.DialogMessageId
}
