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
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/media/media"
)

// dfsProfilePhotoRequest is the dfs upload for a profile photo.
// Video-only and emoji-markup avatars have no still file. Dropping the
// markup here made emoji avatars fail before dfs could store them.
func dfsProfilePhotoRequest(in *media.TLMediaUploadProfilePhotoFile) (*dfs.TLDfsUploadProfilePhotoFileV2, error) {
	if in == nil || (in.GetFile() == nil && in.GetVideo() == nil && in.GetVideoEmojiMarkup() == nil) {
		return nil, mtproto.ErrMediaInvalid
	}
	return &dfs.TLDfsUploadProfilePhotoFileV2{
		Creator:          in.GetOwnerId(),
		File:             in.GetFile(),
		Video:            in.GetVideo(),
		VideoStartTs:     in.GetVideoStartTs(),
		VideoEmojiMarkup: in.GetVideoEmojiMarkup(),
	}, nil
}

// MediaUploadProfilePhotoFile
// media.uploadProfilePhotoFile flags:# owner_id:long file:flags.0?InputFile video:flags.1?InputFile video_start_ts:flags.2?double video_emoji_markup:flags.3?VideoSize = Photo;
func (c *MediaCore) MediaUploadProfilePhotoFile(in *media.TLMediaUploadProfilePhotoFile) (*mtproto.Photo, error) {
	req, err := dfsProfilePhotoRequest(in)
	if err != nil {
		c.Logger.Errorf("media.uploadProfilePhotoFile - error: file is nil")
		return nil, err
	}

	photo, err := c.svcCtx.Dao.DfsClient.DfsUploadProfilePhotoFileV2(c.ctx, req)
	if err != nil {
		c.Logger.Error("media.uploadProfilePhotoFile - error: %v", err.Error())
		return nil, err
	}

	hasVideo := len(photo.GetVideoSizes()) > 0

	if err = c.svcCtx.Dao.SavePhotoSizeV2(c.ctx, photo.GetId(), photo.GetSizes()); err != nil {
		c.Logger.Error("media.uploadProfilePhotoFile - error: %v", err.Error())
		return nil, err
	}
	if hasVideo {
		_ = c.svcCtx.Dao.SaveVideoSizeV2(c.ctx, photo.GetId(), photo.GetVideoSizes())
	}

	_ = c.svcCtx.SavePhotoV2(c.ctx,
		photo.GetId(),
		photo.GetAccessHash(),
		photo.GetHasStickers(),
		hasVideo,
		in.GetFile().GetName())

	return photo, nil
}
