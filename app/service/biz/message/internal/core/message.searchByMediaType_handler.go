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
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageSearchByMediaType
// message.searchByMediaType user_id:long peer:PeerUtil media_type:int offset:int limit:int = Vector<MessageBox>;
func (c *MessageCore) MessageSearchByMediaType(in *message.TLMessageSearchByMediaType) (*mtproto.MessageBoxList, error) {
	var (
		boxList []*mtproto.MessageBox
	)

	switch {
	case in.GetPeerType() == message.SearchGlobalPeerType && in.GetPeerId() == 0:
		boxList = c.searchGlobalMedia(in.UserId, in.MediaType, in.Offset, in.Limit)
	case in.GetMediaType() == mtproto.MEDIA_PHONE_CALL:
		boxList = c.searchByPhoneCall(in.UserId, in.Offset, in.Limit)
	case in.GetMediaType() == mtproto.MEDIA_PHOTOVIDEO:
		boxList = c.searchByPhotoVideoMediaType(in.UserId, in.PeerType, in.PeerId, in.Offset, in.Limit)
	default:
		boxList = c.searchByMediaType(in.UserId, in.PeerType, in.PeerId, in.MediaType, in.Offset, in.Limit)
	}

	return mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{
		BoxList: boxList,
	}).To_MessageBoxList(), nil
}

func (c *MessageCore) searchByPhotoVideoMediaType(
	userId int64,
	peerType int32,
	peerId int64,
	offset, limit int32) (boxList []*mtproto.MessageBox) {

	var (
		dialogId = mtproto.MakeDialogId(userId, peerType, peerId)
	)

	_, _ = c.svcCtx.Dao.MessagesDAO.SelectByPhotoVideoMediaTypeWithCB(
		c.ctx,
		userId,
		dialogId.A,
		dialogId.B,
		offset,
		limit,
		func(sz, i int, v *dataobject.MessagesDO) {
			boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, userId, v))
		})

	if boxList == nil {
		boxList = []*mtproto.MessageBox{}
	}

	return
}

func (c *MessageCore) searchByMediaType(
	userId int64,
	peerType int32,
	peerId int64,
	mediaType int32,
	offset, limit int32) (boxList []*mtproto.MessageBox) {

	var (
		dialogId = mtproto.MakeDialogId(userId, peerType, peerId)
	)

	types := searchMediaTypes(mediaType)
	if len(types) == 1 {
		_, _ = c.svcCtx.Dao.MessagesDAO.SelectByMediaTypeWithCB(
			c.ctx,
			userId,
			dialogId.A,
			dialogId.B,
			types[0],
			offset,
			limit,
			func(sz, i int, v *dataobject.MessagesDO) {
				boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, userId, v))
			})
	} else {
		// One query per stored kind, then the newest `limit` across them.
		// A voice note written as MEDIA_VOICE_FILE still shows up in a
		// search that asks for the old MEDIA_AUDIO bucket, and the reverse.
		var parts [][]*mtproto.MessageBox
		for _, kind := range types {
			var part []*mtproto.MessageBox
			_, _ = c.svcCtx.Dao.MessagesDAO.SelectByMediaTypeWithCB(
				c.ctx,
				userId,
				dialogId.A,
				dialogId.B,
				kind,
				offset,
				limit,
				func(sz, i int, v *dataobject.MessagesDO) {
					part = append(part, c.svcCtx.Dao.MakeMessageBox(c.ctx, userId, v))
				})
			parts = append(parts, part)
		}
		boxList = mergeNewestBoxes(parts, limit)
	}

	if boxList == nil {
		boxList = []*mtproto.MessageBox{}
	}

	return
}

// mergeNewestBoxes keeps the newest distinct boxes, at most limit.
// Each part is already limited to `limit` rows of one filter type, so the
// global newest `limit` are all present.
func mergeNewestBoxes(parts [][]*mtproto.MessageBox, limit int32) []*mtproto.MessageBox {
	seen := map[int32]struct{}{}
	var merged []*mtproto.MessageBox
	for _, part := range parts {
		for _, box := range part {
			if box == nil {
				continue
			}
			id := box.GetMessageId()
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			merged = append(merged, box)
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].GetMessageId() > merged[j].GetMessageId()
	})
	if limit > 0 && int32(len(merged)) > limit {
		merged = merged[:limit]
	}
	return merged
}

func (c *MessageCore) searchGlobalMedia(userId int64, mediaType, offset, limit int32) (boxList []*mtproto.MessageBox) {
	types := searchMediaTypes(mediaType)
	matchURL := false
	switch mediaType {
	case mtproto.MEDIA_PHOTOVIDEO:
		types = []int32{mtproto.MEDIA_PHOTOVIDEO, mtproto.MEDIA_PHOTOS_ONLY, mtproto.MEDIA_VIDEOS_ONLY}
	case mtproto.MEDIA_URL:
		matchURL = true
	}
	_, _ = c.svcCtx.Dao.MessagesDAO.SearchGlobalByMediaWithCB(
		c.ctx,
		userId,
		offset,
		limit,
		types,
		matchURL,
		func(sz, i int, v *dataobject.MessagesDO) {
			boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, userId, v))
		})
	if boxList == nil {
		boxList = []*mtproto.MessageBox{}
	}
	return
}

func (c *MessageCore) searchByPhoneCall(userId int64, offset, limit int32) (boxList []*mtproto.MessageBox) {
	_, _ = c.svcCtx.Dao.MessagesDAO.SelectPhoneCallListWithCB(
		c.ctx,
		userId,
		mtproto.MEDIA_PHONE_CALL,
		offset,
		limit,
		func(sz, i int, v *dataobject.MessagesDO) {
			boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, userId, v))
		})

	if boxList == nil {
		boxList = []*mtproto.MessageBox{}
	}

	return
}
