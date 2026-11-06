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
	"bytes"
	"fmt"
	"image"
	"math/rand"
	"time"

	"github.com/teamgram/marmota/pkg/bytes2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
	"github.com/teamgram/teamgram-server/app/service/mediablob"
)

func makeMp4DocumentAttributes(attrs []*mtproto.DocumentAttribute, videoW, videoH int32) []*mtproto.DocumentAttribute {
	attributes := make([]*mtproto.DocumentAttribute, 0, len(attrs))
	for _, attr := range attrs {
		switch attr.GetPredicateName() {
		case mtproto.Predicate_documentAttributeFilename:
			attributes = append(attributes, attr)
		case mtproto.Predicate_documentAttributeVideo:
			w, h := attr.W, attr.H
			if !attr.RoundMessage && videoW > 0 && videoH > 0 {
				w, h = videoW, videoH
			}
			attributes = append(attributes, mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
				RoundMessage:      attr.RoundMessage,
				SupportsStreaming: true,
				Nosound:           attr.Nosound,
				Duration:          attr.Duration, // gif.mp4's duration
				Duration_INT32:    attr.Duration_INT32,
				Duration_FLOAT64:  attr.Duration_FLOAT64,
				W:                 w,
				H:                 h,
				PreloadPrefixSize: attr.PreloadPrefixSize,
				VideoStartTs:      attr.VideoStartTs,
				VideoCodec:        attr.VideoCodec,
			}).To_DocumentAttribute())
		case mtproto.Predicate_documentAttributeAnimated:
			attributes = append(attributes, attr)
		}
	}
	return attributes
}

// DfsUploadMp4DocumentMedia
// dfs.uploadMp4DocumentMedia creator:long media:InputMedia = Document;
func (c *DfsCore) DfsUploadMp4DocumentMedia(in *dfs.TLDfsUploadMp4DocumentMedia) (*mtproto.Document, error) {
	var (
		documentId = c.svcCtx.Dao.IDGenClient2.NextId(c.ctx)

		file      = in.GetMedia().GetFile()
		creatorId = in.GetCreator()
		media     = in.GetMedia()

		ext        = model.GetFileExtName(in.GetMedia().GetFile().GetName())
		extType    = model.GetStorageFileTypeConstructor(ext)
		accessHash = int64(extType)<<32 | int64(rand.Uint32())
	)

	// Read the upload directly. Fetching /dfs/file/<client name>.mp4 404s
	// (invalid fileName x.mp4) and leaves the GIF without a thumb.
	mp4Bytes, err := c.uploadedBytes(creatorId, file.GetId_INT64(), file.GetParts())
	if err != nil {
		c.Logger.Errorf("dfs.uploadDocumentFile - error: %v", err)
		return nil, err
	}
	dispW, dispH, rot := probeVideoDisplay(c.svcCtx.FFmpegUtil, mp4Bytes)
	videoW, videoH := chosenAttributeSize(media.GetAttributes(), dispW, dispH, rot)
	fileInfo, err := c.svcCtx.Dao.GetFileInfo(c.ctx, creatorId, file.Id_INT64)
	if err != nil {
		c.Logger.Errorf("dfs.uploadDocumentFile - error: %v", err)
		return nil, err
	}
	claim, err := c.storeDocumentBytes(c.ctx, documentId, mp4Bytes, true)
	if err != nil {
		c.Logger.Errorf("dfs.PutDocumentFile - error: %v", err)
		c.abandonClaim(c.ctx, mediablob.KindDocument, documentId)
		return nil, err
	}
	if !claim.Reused {
		c.svcCtx.Dao.SetCacheFileInfo(c.ctx, documentId, fileInfo)
	}
	thumbs, _ := c.reusedPhotoSizes(c.ctx, documentId, claim)
	if thumbs == nil {
		thumbs = c.mp4Thumbs(documentId, mp4Bytes, rot, videoW, videoH, claim)
	}
	return c.plainMp4Document(documentId, accessHash, claim, media, videoW, videoH, thumbs, mp4Bytes), nil
}

// mp4Thumbs builds the JPEG preview. A frame ffmpeg cannot turn into a JPEG
// must not fail the upload: the document is stored without a thumb.
func (c *DfsCore) mp4Thumbs(documentId int64, mp4Bytes []byte, rot int, videoW, videoH int32, claim mediablob.Claim) []*mtproto.PhotoSize {
	thumbData, err := c.svcCtx.FFmpegUtil.GetFirstFrameByPipe(mp4Bytes)
	if len(thumbData) == 0 || err != nil {
		return nil
	}
	thumb, err := imaging.Decode(bytes.NewReader(thumbData))
	if err != nil {
		c.Logger.Errorf("dfs.uploadMp4DocumentMedia - thumb: %v", err)
		return nil
	}
	// A coded landscape frame of a portrait video must not become the
	// bubble size. Clients use this thumb's w/h and center-crop the
	// picture into that box.
	thumb = imaging.MatchDisplayAspect(thumb, rot, int(videoW), int(videoH))
	stripped := bytes2.NewBuffer(make([]byte, 0, 4096))
	if thumb.Bounds().Dx() >= thumb.Bounds().Dy() {
		err = imaging.EncodeStripped(stripped, imaging.Resize(thumb, 40, 0), 30)
	} else {
		err = imaging.EncodeStripped(stripped, imaging.Resize(thumb, 0, 40), 30)
	}
	if err != nil {
		c.Logger.Errorf("dfs.uploadMp4DocumentMedia - stripped thumb: %v", err)
		return nil
	}

	var (
		mThumbData = bytes2.NewBuffer(make([]byte, 0, len(thumbData)))
		mThumb     image.Image
	)
	if thumb.Bounds().Dx() >= thumb.Bounds().Dy() {
		mThumb = imaging.Resize(thumb, 320, 0)
	} else {
		mThumb = imaging.Resize(thumb, 0, 320)
	}
	if err = imaging.EncodeJpeg(mThumbData, mThumb); err != nil {
		c.Logger.Errorf("dfs.uploadMp4DocumentMedia - jpeg thumb: %v", err)
		return nil
	}

	path := fmt.Sprintf("%s/%d.dat", mtproto.PhotoSZMediumType, documentId)
	if shared := c.originSizePaths(c.ctx, claim)[mtproto.PhotoSZMediumType]; shared != "" {
		path = shared
	} else if _, err = c.svcCtx.Dao.PutPhotoFile(c.ctx, path, mThumbData.Bytes()); err != nil {
		c.Logger.Errorf("dfs.uploadMp4DocumentMedia - thumb put: %v", err)
		return nil
	}
	c.rememberPhotoPath(c.ctx, documentId, mtproto.PhotoSZMediumType, path)
	return []*mtproto.PhotoSize{
		mtproto.MakeTLPhotoStrippedSize(&mtproto.PhotoSize{
			Type:  mtproto.PhotoSZStrippedType,
			Bytes: stripped.Bytes(),
		}).To_PhotoSize(),
		mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{
			Type:  mtproto.PhotoSZMediumType,
			W:     int32(mThumb.Bounds().Dx()),
			H:     int32(mThumb.Bounds().Dy()),
			Size2: int32(len(mThumbData.Bytes())),
		}).To_PhotoSize(),
	}
}

func (c *DfsCore) plainMp4Document(documentId, accessHash int64, claim mediablob.Claim, media *mtproto.InputMedia, videoW, videoH int32, thumbs []*mtproto.PhotoSize, mp4Bytes []byte) *mtproto.Document {
	document := mtproto.MakeTLDocument(&mtproto.Document{
		Id:            documentId,
		AccessHash:    accessHash,
		FileReference: []byte{},
		Date:          int32(time.Now().Unix()),
		MimeType:      "video/mp4",
		Size2:         claim.Size,
		Size2_INT32:   int32(claim.Size),
		Size2_INT64:   claim.Size,
		Thumbs:        thumbs,
		VideoThumbs:   nil,
		DcId:          1,
		Attributes:    makeMp4DocumentAttributes(media.GetAttributes(), videoW, videoH),
	}).To_Document()
	return finishGifDocument(document, mp4Bytes)
}
