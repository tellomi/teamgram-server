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
	"crypto/md5"
	"fmt"
	"math/rand"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"

	"github.com/minio/minio-go/v7"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	profilePhotoFile = iota + 1
	profilePhotoVideo
	profilePhotoEmoji
)

// profilePhotoID allocates the photo id for an emoji or sticker markup avatar.
// Those avatars have no uploaded bytes. Tests replace this.
var profilePhotoID = defaultProfilePhotoID

func defaultProfilePhotoID(c *DfsCore) (int64, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return 0, mtproto.ErrInternalServerError
	}
	id := c.svcCtx.Dao.IDGenClient2.NextId(c.ctx)
	if id == 0 {
		return 0, mtproto.ErrInternalServerError
	}
	return id, nil
}

// profilePhotoSource picks the avatar payload.
// A video is the animated avatar, including when a still file is sent with it.
// A file alone is a still photo. Emoji and sticker markup have neither file.
func profilePhotoSource(in *dfs.TLDfsUploadProfilePhotoFileV2) (kind int, src *mtproto.InputFile, err error) {
	if in == nil {
		return 0, nil, mtproto.ErrInputRequestInvalid
	}
	if video := in.GetVideo(); video != nil {
		return profilePhotoVideo, video, nil
	}
	if file := in.GetFile(); file != nil {
		return profilePhotoFile, file, nil
	}
	if in.GetVideoEmojiMarkup() != nil {
		return profilePhotoEmoji, nil, nil
	}
	return 0, nil, mtproto.ErrInputRequestInvalid
}

// DfsUploadProfilePhotoFileV2
// dfs.uploadProfilePhotoFileV2 flags:# creator:long file:flags.0?InputFile video:flags.1?InputFile video_start_ts:flags.2?double video_emoji_markup:flags.3?VideoSize = Photo;
func (c *DfsCore) DfsUploadProfilePhotoFileV2(in *dfs.TLDfsUploadProfilePhotoFileV2) (*mtproto.Photo, error) {
	kind, src, err := profilePhotoSource(in)
	if err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		return nil, err
	}

	switch kind {
	case profilePhotoVideo:
		if err = model.CheckFileParts(src.Parts); err != nil {
			c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
			return nil, err
		}
		photo, err := c.uploadVideoSizeListV2(in.GetCreator(), src, in.GetVideoStartTs().GetValue())
		if err != nil {
			c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
			return nil, err
		}
		return photo, nil
	case profilePhotoFile:
		if err = model.CheckFileParts(src.Parts); err != nil {
			c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
			return nil, err
		}
		photo, err := c.uploadPhotoSizeListV2(in.GetCreator(), src, true)
		if err != nil {
			c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
			return nil, err
		}
		return photo, nil
	case profilePhotoEmoji:
		id, err := profilePhotoID(c)
		if err != nil {
			c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
			return nil, err
		}
		photo, err := emojiMarkupProfilePhoto(id, in.GetVideoEmojiMarkup())
		if err != nil {
			c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
			return nil, err
		}
		return photo, nil
	default:
		c.Logger.Errorf("dfs.uploadPhotoFile - ErrInputRequestInvalid")
		return nil, mtproto.ErrInputRequestInvalid
	}
}

// emojiMarkupProfilePhoto is an avatar the client draws from a custom emoji
// or a sticker. photos.uploadProfilePhoto sends videoSizeEmojiMarkup or
// videoSizeStickerMarkup and no file. The photo carries that VideoSize and
// no uploaded image.
func emojiMarkupProfilePhoto(id int64, markup *mtproto.VideoSize) (*mtproto.Photo, error) {
	vs, err := copyEmojiMarkup(markup)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLPhoto(&mtproto.Photo{
		Id:            id,
		HasStickers:   false,
		AccessHash:    int64(rand.Uint32()),
		FileReference: []byte{},
		Date:          int32(time.Now().Unix()),
		Sizes:         []*mtproto.PhotoSize{},
		VideoSizes:    []*mtproto.VideoSize{vs},
		DcId:          1,
	}).To_Photo(), nil
}

func copyEmojiMarkup(markup *mtproto.VideoSize) (*mtproto.VideoSize, error) {
	if markup == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	colors := markup.GetBackgroundColors()
	if len(colors) < 1 || len(colors) > 4 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	copied := append([]int32(nil), colors...)
	switch markup.GetPredicateName() {
	case mtproto.Predicate_videoSizeEmojiMarkup:
		if markup.GetEmojiId() == 0 {
			return nil, mtproto.ErrInputRequestInvalid
		}
		return mtproto.MakeTLVideoSizeEmojiMarkup(&mtproto.VideoSize{
			EmojiId:          markup.GetEmojiId(),
			BackgroundColors: copied,
		}).To_VideoSize(), nil
	case mtproto.Predicate_videoSizeStickerMarkup:
		if markup.GetStickerId() == 0 || markup.GetStickerset() == nil {
			return nil, mtproto.ErrInputRequestInvalid
		}
		return mtproto.MakeTLVideoSizeStickerMarkup(&mtproto.VideoSize{
			Stickerset:       markup.GetStickerset(),
			StickerId:        markup.GetStickerId(),
			BackgroundColors: copied,
		}).To_VideoSize(), nil
	default:
		return nil, mtproto.ErrInputRequestInvalid
	}
}

func (c *DfsCore) uploadVideoSizeListV2(creatorId int64, video *mtproto.InputFile, videoStartTs float64) (photo *mtproto.Photo, err error) {
	var (
		videoMp4Data []byte
		photoData    []byte
		// photoImg     image.Image
		r *dao.SSDBReader
	)

	r, err = c.svcCtx.Dao.OpenFile(c.ctx, creatorId, video.Id_INT64, video.Parts)
	if err != nil {
		c.Logger.Errorf("dfs.uploadVideoSizeList - %v", err)
		return nil, mtproto.ErrMediaInvalid
	}

	videoMp4Data, err = r.ReadAll(c.ctx)
	if err != nil {
		c.Logger.Errorf("dfs.uploadVideoSizeList - %v", err)
		return nil, mtproto.ErrMediaInvalid
	}

	if len(video.Md5Checksum) > 0 {
		digest := fmt.Sprintf("%x", md5.Sum(videoMp4Data))
		if digest != video.Md5Checksum {
			c.Logger.Errorf("dfs.uploadPhotoFile - (%s, %s) error: %v", digest, video.Md5Checksum, err)
			return nil, mtproto.ErrCheckSumInvalid
		}
	}

	var (
		// videoId = idgen.GetUUID()
		// ext         = model.GetFileExtName(video.GetName())
		// extType     = model.GetStorageFileTypeConstructor(ext)
		tmpFileName = fmt.Sprintf("http://127.0.0.1:11701/dfs/file/%d_%d.mp4", creatorId, video.GetId_INT64())
	)

	videoMp4Data, _, err = c.svcCtx.FFmpegUtil.ConvertToMp4ByPipe(tmpFileName, 800, 800)
	if err != nil {
		c.Logger.Errorf("uploadVideoSizeList - error: %v", err)
		return nil, err
	}

	// getFirstFrame
	photoData, err = c.svcCtx.FFmpegUtil.GetFirstFrameByPipe(videoMp4Data)
	if err != nil {
		c.Logger.Errorf("getFirstFrameByPipe - error: %v", err)
		return nil, err
	}

	var (
		photoId  = c.svcCtx.Dao.IDGenClient2.NextId(c.ctx)
		sizeList = make([]*mtproto.PhotoSize, 0, 3)
		// extType  = model.GetStorageFileTypeConstructor(ext)
		fileSize minio.UploadInfo
		//ext         = model.GetFileExtName(video.GetName())
		ext        = ".jpg"
		extType    = model.GetStorageFileTypeConstructor(ext)
		accessHash = int64(extType)<<32 | int64(rand.Uint32())
	)

	err = imaging.ReSizeImage(photoData, ext, true, func(szType string, localId int, w, h int32, b []byte) error {
		// secretId := int64(extType)<<32 | int64(rand.Uint32())
		path := fmt.Sprintf("%s/%d.dat", szType, photoId)
		// c.Logger.Debugf("path: %s", path)
		fileSize, err = c.putProfilePhoto(path, b)
		if err != nil {
			c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
			return err
		}

		sizeList = append(sizeList, mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{
			Type:  szType,
			W:     w,
			H:     h,
			Size2: int32(fileSize.Size),
		}).To_PhotoSize())

		return nil
	})

	if len(sizeList) == 0 {
		err = mtproto.ErrImageProcessFailed
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		return nil, err
	}

	// ext = model.GetFileExtName(video.GetName())
	// extType = model.GetStorageFileTypeConstructor(ext)
	// secretId := int64(extType)<<32 | int64(rand.Uint32())
	path := fmt.Sprintf("%s/%d.dat", mtproto.VideoSZVType, photoId)
	_, err = c.putProfileVideo(path, videoMp4Data)
	if err != nil {
		c.Logger.Errorf("uploadVideoSizeList - error: %v", err)
		return nil, err
	}

	videoSize := mtproto.MakeTLVideoSize(&mtproto.VideoSize{
		Type:         mtproto.VideoSZVType,
		W:            800,
		H:            800,
		Size2:        int32(len(videoMp4Data)),
		VideoStartTs: nil,
	}).To_VideoSize()

	if videoStartTs > 0 {
		videoSize.VideoStartTs = &wrapperspb.DoubleValue{Value: videoStartTs}
	}

	return mtproto.MakeTLPhoto(&mtproto.Photo{
		Id:            photoId,
		HasStickers:   false,
		AccessHash:    accessHash,
		FileReference: []byte{},
		Date:          int32(time.Now().Unix()),
		Sizes:         sizeList,
		VideoSizes:    []*mtproto.VideoSize{videoSize},
		DcId:          1,
	}).To_Photo(), nil
}

func (c *DfsCore) uploadPhotoSizeListV2(creatorId int64, file *mtproto.InputFile, isABC bool) (photo *mtproto.Photo, err error) {
	var (
		fileSize  minio.UploadInfo
		cacheData []byte
		r         *dao.SSDBReader
	)

	r, err = c.svcCtx.Dao.OpenFile(c.ctx, creatorId, file.Id_INT64, file.Parts)
	if err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		return nil, mtproto.ErrMediaInvalid
	}

	cacheData, err = r.ReadAll(c.ctx)
	if err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		return nil, mtproto.ErrMediaInvalid
	}

	if len(file.Md5Checksum) > 0 {
		digest := fmt.Sprintf("%x", md5.Sum(cacheData))
		if digest != file.Md5Checksum {
			c.Logger.Errorf("dfs.uploadPhotoFile - (%s, %s) error: %v", digest, file.Md5Checksum, err)
			return nil, mtproto.ErrCheckSumInvalid
		}
	}

	var (
		photoId    = c.svcCtx.Dao.IDGenClient2.NextId(c.ctx)
		ext        = model.GetFileExtName(file.GetName())
		sizeList   = make([]*mtproto.PhotoSize, 0, 3)
		extType    = model.GetStorageFileTypeConstructor(ext)
		accessHash = int64(extType)<<32 | int64(rand.Uint32())
	)

	_, err = c.putProfilePhoto(fmt.Sprintf("0/%d.dat", photoId), cacheData)
	if err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
	}

	err = imaging.ReSizeImage(cacheData, ext, isABC, func(szType string, localId int, w, h int32, b []byte) error {
		// secretId := int64(extType)<<32 | int64(rand.Uint32())
		path := fmt.Sprintf("%s/%d.dat", szType, photoId)
		// c.Logger.Debugf("path: %s", path)
		fileSize, err = c.putProfilePhoto(path, b)
		if err != nil {
			c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
			return err
		}

		sizeList = append(sizeList, mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{
			Type:  szType,
			W:     w,
			H:     h,
			Size2: int32(fileSize.Size),
		}).To_PhotoSize())

		return nil
	})

	if len(sizeList) == 0 {
		err = mtproto.ErrImageProcessFailed
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		return nil, err
	}

	return mtproto.MakeTLPhoto(&mtproto.Photo{
		Id:            photoId,
		HasStickers:   false,
		AccessHash:    accessHash,
		FileReference: []byte{},
		Date:          int32(time.Now().Unix()),
		Sizes:         sizeList,
		VideoSizes:    nil,
		DcId:          1,
	}).To_Photo(), nil
}
