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
	"math"

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

// countAudioKind counts legacy MEDIA_AUDIO rows. Voice notes and round
// videos were both written into that one column; the document attribute
// says which is which. voice selects voice notes, otherwise round videos.
func (c *MessageCore) countAudioKind(userID, dialogA, dialogB int64, voice bool) int32 {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessagesDAO == nil {
		return 0
	}
	const page = 200
	var n int32
	offset := int32(math.MaxInt32)
	for {
		rows, err := c.svcCtx.Dao.MessagesDAO.SelectByMediaType(
			c.ctx, userID, dialogA, dialogB, mtproto.MEDIA_AUDIO, offset, page)
		if err != nil || len(rows) == 0 {
			break
		}
		next := offset
		for i := range rows {
			if rows[i].UserMessageBoxId < next {
				next = rows[i].UserMessageBoxId
			}
			box := c.svcCtx.Dao.MakeMessageBox(c.ctx, userID, &rows[i])
			msg := box.GetMessage()
			if voice && mtproto.IsVoiceMessage(msg) {
				n++
			}
			if !voice && mtproto.IsRoundVideoMessage(msg) {
				n++
			}
		}
		if len(rows) < page || next >= offset {
			break
		}
		offset = next
	}
	return n
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
