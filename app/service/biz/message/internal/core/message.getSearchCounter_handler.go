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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageGetSearchCounter
// message.getSearchCounter user_id:long peer_type:int peer_id:long media_type:int = Int32;
func (c *MessageCore) MessageGetSearchCounter(in *message.TLMessageGetSearchCounter) (*mtproto.Int32, error) {
	dialogID := mtproto.MakeDialogId(in.UserId, in.PeerType, in.PeerId)
	var n int32
	switch in.MediaType {
	case mtproto.MEDIA_PHOTOVIDEO:
		// Photos are stored as MEDIA_PHOTOS_ONLY and videos as
		// MEDIA_VIDEOS_ONLY. The combined type is a legacy bucket.
		n = c.countMediaTypes(in.UserId, dialogID.A, dialogID.B,
			mtproto.MEDIA_PHOTOVIDEO, mtproto.MEDIA_PHOTOS_ONLY, mtproto.MEDIA_VIDEOS_ONLY)
	case mtproto.MEDIA_VOICE_FILE:
		n = c.countMediaTypes(in.UserId, dialogID.A, dialogID.B, mtproto.MEDIA_VOICE_FILE)
		n += c.countAudioKind(in.UserId, dialogID.A, dialogID.B, true)
	case mtproto.MEDIA_ROUND_FILE:
		n = c.countMediaTypes(in.UserId, dialogID.A, dialogID.B, mtproto.MEDIA_ROUND_FILE)
		n += c.countAudioKind(in.UserId, dialogID.A, dialogID.B, false)
	case mtproto.MEDIA_AUDIO:
		n = c.countMediaTypes(in.UserId, dialogID.A, dialogID.B,
			mtproto.MEDIA_AUDIO, mtproto.MEDIA_VOICE_FILE, mtproto.MEDIA_ROUND_FILE)
	case mtproto.MEDIA_PINNED:
		n = c.countPinned(in.UserId, dialogID.A, dialogID.B)
	default:
		n = c.countMediaTypes(in.UserId, dialogID.A, dialogID.B, in.MediaType)
	}
	return &mtproto.Int32{V: n}, nil
}

func (c *MessageCore) countMediaTypes(userID, dialogA, dialogB int64, types ...int32) int32 {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.CommonDAO == nil || c.svcCtx.Dao.MessagesDAO == nil {
		return 0
	}
	table := c.svcCtx.Dao.MessagesDAO.CalcTableName(userID)
	var n int32
	for _, kind := range types {
		sz := c.svcCtx.Dao.CommonDAO.CalcSize(c.ctx, table, map[string]interface{}{
			"user_id":             userID,
			"dialog_id1":          dialogA,
			"dialog_id2":          dialogB,
			"message_filter_type": kind,
			"deleted":             0,
		})
		n += int32(sz)
	}
	return n
}

// countAudioKind counts MEDIA_AUDIO rows of one kind. Voice notes and round
// videos are still stored in that one column, so messages.search on an
// older build keeps listing them. The kind is a count(id) on the same
// table photos use, not a separate paged read of message_data.
func (c *MessageCore) countAudioKind(userID, dialogA, dialogB int64, voice bool) int32 {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.CommonDAO == nil || c.svcCtx.Dao.MessagesDAO == nil {
		return 0
	}
	return int32(c.svcCtx.Dao.CommonDAO.CalcSizeByWhere(
		c.ctx,
		c.svcCtx.Dao.MessagesDAO.CalcTableName(userID),
		audioKindWhere(userID, dialogA, dialogB, voice),
	))
}

func (c *MessageCore) countPinned(userID, dialogA, dialogB int64) int32 {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.CommonDAO == nil || c.svcCtx.Dao.MessagesDAO == nil {
		return 0
	}
	sz := c.svcCtx.Dao.CommonDAO.CalcSize(
		c.ctx,
		c.svcCtx.Dao.MessagesDAO.CalcTableName(userID),
		map[string]interface{}{
			"user_id":    userID,
			"dialog_id1": dialogA,
			"dialog_id2": dialogB,
			"pinned":     1,
			"deleted":    0,
		})
	return int32(sz)
}
