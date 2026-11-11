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
	"crypto/md5"
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
	"github.com/teamgram/teamgram-server/pkg/uploadledger"
)

const (
	GifNeedConvertSize = 10240
)

// DfsUploadGifDocumentMedia
// dfs.uploadGifDocumentMedia creator:long media:InputMedia = Document;
func (c *DfsCore) DfsUploadGifDocumentMedia(in *dfs.TLDfsUploadGifDocumentMedia) (*mtproto.Document, error) {
	var (
		media    = in.GetMedia()
		file     = in.GetMedia().GetFile()
		thumb    = media.GetThumb()
		err      error
		document *mtproto.Document
	)

	if media == nil {
		err = mtproto.ErrMediaEmpty
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
		return nil, err
	}

	if file == nil {
		err = mtproto.ErrMediaInvalid
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
		return nil, err
	}

	if err := model.CheckFileParts(file.Parts); err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - %v", err)
		return nil, err
	}

	if thumb != nil {
		if err = model.CheckFileParts(file.Parts); err != nil {
			c.Logger.Errorf("dfs.uploadGifDocumentMedia - %v", err)
			return nil, err
		}
	}

	opened, err2 := c.svcCtx.Dao.OpenFile(c.ctx, in.GetCreator(), file.GetId_INT64(), file.Parts)
	if err2 != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err2)
		return nil, partMissOr(err2, mtproto.ErrMediaInvalid)
	}
	fileInfo := opened.DfsFileInfo

	if fileInfo.GetFileSize() < GifNeedConvertSize {
		document, err = c.uploadGifMedia(in.GetCreator(), media, fileInfo)
	} else {
		if media.GetThumb() != nil {
			document, err = c.uploadHasThumbGifMp4Media(in.GetCreator(), media)
		} else {
			document, err = c.uploadGifMp4Media(in.GetCreator(), media)
		}
	}

	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
		return nil, err
	}

	return document, nil
}

func (c *DfsCore) uploadGifMedia(creatorId int64, media *mtproto.InputMedia, fileInfo *model.DfsFileInfo) (*mtproto.Document, error) {
	var (
		documentId = c.nextID()
		ext        = model.GetFileExtName(media.GetFile().GetName())
		extType    = model.GetStorageFileTypeConstructor(ext)
		accessHash = int64(extType)<<32 | int64(rand.Uint32())
		file       = media.GetFile()
	)

	gifData, err := c.svcCtx.Dao.NewSSDBReader(fileInfo).ReadAll(c.ctx)
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
		return nil, err
	}

	// build photoStrippedSize
	gifThumb, err := imaging.Decode(bytes.NewReader(gifData))
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
		return nil, err
	}

	stored, err := c.storeDocumentBytes(uploadledger.WithUser(c.ctx, creatorId), documentId, gifData, false)
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
		c.abandonClaim(c.ctx, mediablob.KindDocument, documentId)
		return nil, err
	}
	szList, err := c.uploadGifThumb(documentId, gifThumb, stored)
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
		c.abandonStored(creatorId, documentId, stored)
		return nil, err
	}

	// build document
	document := mtproto.MakeTLDocument(&mtproto.Document{
		Id:            documentId,
		AccessHash:    accessHash,
		FileReference: []byte{}, // TODO(@benqi): gen file_reference
		Date:          int32(time.Now().Unix()),
		MimeType:      "image/gif",
		Size2:         stored.Size,
		Size2_INT32:   int32(stored.Size),
		Size2_INT64:   stored.Size,
		Thumbs:        szList,
		VideoThumbs:   nil,
		DcId:          1,
		Attributes: []*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeImageSize(&mtproto.DocumentAttribute{
				W: int32(gifThumb.Bounds().Dx()),
				H: int32(gifThumb.Bounds().Dy()),
			}).To_DocumentAttribute(),
			mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
				FileName: file.GetName(),
			}).To_DocumentAttribute(),
		},
	}).To_Document()

	releaseUploadHold(c.ledgerSubject(creatorId), file.GetId_INT64())
	return finishGifDocument(document, gifData), nil
}

func (c *DfsCore) uploadHasThumbGifMp4Media(creatorId int64, media *mtproto.InputMedia) (*mtproto.Document, error) {
	var (
		documentId = c.nextID()
		ext        = model.GetFileExtName(media.GetFile().GetName())
		extType    = model.GetStorageFileTypeConstructor(ext)
		accessHash = int64(extType)<<32 | int64(rand.Uint32())
	)

	// 1. thumb
	thumbFile := media.GetThumb()

	if err := model.CheckFileParts(thumbFile.Parts); err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		return nil, err
	}

	r, err := c.svcCtx.Dao.OpenFile(c.ctx, creatorId, thumbFile.Id_INT64, thumbFile.Parts)
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - %v", err)
		return nil, partMissOr(err, mtproto.ErrMediaInvalid)
	}

	thumbFileData, err := r.ReadAll(c.ctx)
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - %v", err)
		return nil, partMissOr(err, mtproto.ErrMediaInvalid)
	}

	if len(thumbFile.Md5Checksum) > 0 {
		digest := fmt.Sprintf("%x", md5.Sum(thumbFileData))
		if digest != thumbFile.Md5Checksum {
			c.Logger.Errorf("dfs.uploadGifDocumentMedia - (%s, %s) error: %v", digest, thumbFile.Md5Checksum, err)
			return nil, mtproto.ErrCheckSumInvalid
		}
	}

	// TODO(@benqi): if x or y < 320
	thumb, err := imaging.Decode(bytes.NewReader(thumbFileData))
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - %v", err)
		return nil, mtproto.ErrMediaInvalid
	}

	disp, _ := gifDisplayThumb(thumb)
	dstW := int32(disp.Bounds().Dx())
	if dstW%2 != 0 {
		dstW += 1
	}
	dstH := int32(disp.Bounds().Dy())
	if dstH%2 != 0 {
		dstH += 1
	}
	raw, err := c.uploadedBytes(creatorId, media.GetFile().GetId_INT64(), media.GetFile().GetParts())
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - %v", err)
		return nil, partMissOr(err, mtproto.ErrMediaInvalid)
	}
	gifMp4Data, duration, err := c.svcCtx.FFmpegUtil.ConvertBytesToMp4(raw, int(dstW), int(dstH))
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - %v", err)
		return nil, err
	}
	stored, err := c.storeDocumentBytes(uploadledger.WithUser(c.ctx, creatorId), documentId, gifMp4Data, false)
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - %v", err)
		c.abandonClaim(c.ctx, mediablob.KindDocument, documentId)
		return nil, err
	}
	szList, err := c.uploadGifThumb(documentId, thumb, stored)
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
		c.abandonStored(creatorId, documentId, stored)
		return nil, err
	}

	// build document
	document := mtproto.MakeTLDocument(&mtproto.Document{
		Id:            documentId,
		AccessHash:    accessHash,
		FileReference: []byte{}, // TODO(@benqi): gen file_reference
		Date:          int32(time.Now().Unix()),
		MimeType:      "video/mp4",
		Size2:         stored.Size,
		Size2_INT32:   int32(stored.Size),
		Size2_INT64:   stored.Size,
		Thumbs:        szList,
		VideoThumbs:   nil,
		DcId:          1,
		Attributes: []*mtproto.DocumentAttribute{
			//mtproto.MakeTLDocumentAttributeImageSize(&mtproto.DocumentAttribute{
			//	W: int32(thumb.Bounds().Dx()),
			//	H: int32(thumb.Bounds().Dy()),
			//}).To_DocumentAttribute(),
			mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
				RoundMessage:      false,
				SupportsStreaming: true,
				Duration:          float64(duration), // gif.mp4's duration
				Duration_INT32:    duration,
				Duration_FLOAT64:  float64(duration),
				W:                 dstW,
				H:                 dstH,
			}).To_DocumentAttribute(),
			mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
				FileName: model.AnimatedFileName(media.GetFile().GetName()),
			}).To_DocumentAttribute(),
			mtproto.MakeTLDocumentAttributeAnimated(&mtproto.DocumentAttribute{}).To_DocumentAttribute(),
		},
	}).To_Document()

	releaseUploadHold(c.ledgerSubject(creatorId), media.GetFile().GetId_INT64())
	return finishGifDocument(document, gifMp4Data), nil
}

func (c *DfsCore) uploadGifMp4Media(creatorId int64, media *mtproto.InputMedia) (*mtproto.Document, error) {
	var (
		documentId = c.nextID()
		ext        = model.GetFileExtName(media.GetFile().GetName())
		extType    = model.GetStorageFileTypeConstructor(ext)
		accessHash = int64(extType)<<32 | int64(rand.Uint32())
	)

	//	// convert gif to mp4
	imgSize := mtproto.GetDocumentAttribute(media.GetAttributes(), mtproto.Predicate_documentAttributeImageSize)
	if imgSize == nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: imgSize empty")
		return nil, mtproto.ErrMediaInvalid
	}

	var (
		dstW int
		dstH int
		isW  = imgSize.GetW() >= imgSize.GetH()
		sz   = mtproto.GetMaxResizeInfo(false, int(imgSize.GetW()), int(imgSize.GetH()))
	)

	if isW {
		if int(imgSize.GetW()) >= mtproto.PhotoSZMediumSize {
			dstW = mtproto.PhotoSZMediumSize
			dstH = -2
		} else {
			dstW = sz.Size
			dstH = -2
		}
	} else {
		if int(imgSize.GetH()) >= mtproto.PhotoSZMediumSize {
			dstH = mtproto.PhotoSZMediumSize
			dstW = -2
		} else {
			dstH = sz.Size
			dstW = -2
		}
	}

	raw, err := c.uploadedBytes(creatorId, media.GetFile().GetId_INT64(), media.GetFile().GetParts())
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - %v", err)
		return nil, partMissOr(err, mtproto.ErrMediaInvalid)
	}
	gifMp4Data, duration, err := c.svcCtx.FFmpegUtil.ConvertBytesToMp4(raw, dstW, dstH)

	if err != nil {
		c.Logger.Errorf("convertGifToMp4Pipe - error: %v", err)
		return nil, err
	}

	stored, err := c.storeDocumentBytes(uploadledger.WithUser(c.ctx, creatorId), documentId, gifMp4Data, false)
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - %v", err)
		c.abandonClaim(c.ctx, mediablob.KindDocument, documentId)
		return nil, err
	}

	var (
		szList   []*mtproto.PhotoSize
		gifThumb image.Image
	)
	if sizes, ok := c.reusedPhotoSizes(c.ctx, documentId, stored); ok {
		szList = sizes
	} else {
		// getFirstFrame
		thumbData, err := c.svcCtx.FFmpegUtil.GetFirstFrameByPipe(gifMp4Data)
		if err != nil {
			c.Logger.Errorf("convertGifToMp4Pipe - error: %v", err)
			c.abandonStored(creatorId, documentId, stored)
			return nil, err
		}

		// build photoStrippedSize
		gifThumb, err = imaging.Decode(bytes.NewReader(thumbData))
		if err != nil {
			c.Logger.Errorf("convertGifToMp4Pipe - error: %v", err)
			c.abandonStored(creatorId, documentId, stored)
			return nil, err
		}

		szList, err = c.uploadGifThumb(documentId, gifThumb, stored)
		if err != nil {
			c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
			c.abandonStored(creatorId, documentId, stored)
			return nil, err
		}
	}
	frameW, frameH := 0, 0
	if gifThumb != nil {
		frameW, frameH = gifThumb.Bounds().Dx(), gifThumb.Bounds().Dy()
	}
	if frameW == 0 || frameH == 0 {
		frameW, frameH = photoSizeWH(szList)
	}

	// build document
	document := mtproto.MakeTLDocument(&mtproto.Document{
		Id:            documentId,
		AccessHash:    accessHash,
		FileReference: []byte{}, // TODO(@benqi): gen file_reference
		Date:          int32(time.Now().Unix()),
		MimeType:      "video/mp4",
		Size2:         stored.Size,
		Size2_INT32:   int32(stored.Size),
		Size2_INT64:   stored.Size,
		Thumbs:        szList,
		VideoThumbs:   nil,
		DcId:          1,
		Attributes: []*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
				RoundMessage:      false,
				SupportsStreaming: true,
				Nosound:           false,
				Duration:          float64(duration), // gif.mp4's duration
				Duration_INT32:    duration,
				Duration_FLOAT64:  float64(duration),
				W:                 int32(frameW),
				H:                 int32(frameH),
				PreloadPrefixSize: nil,
				VideoStartTs:      nil,
				VideoCodec:        nil,
			}).To_DocumentAttribute(),
			mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
				FileName: model.AnimatedFileName(media.GetFile().GetName()),
			}).To_DocumentAttribute(),
			mtproto.MakeTLDocumentAttributeAnimated(&mtproto.DocumentAttribute{}).To_DocumentAttribute(),
		},
	}).To_Document()

	releaseUploadHold(c.ledgerSubject(creatorId), media.GetFile().GetId_INT64())
	return finishGifDocument(document, gifMp4Data), nil
}

func (c *DfsCore) uploadGifThumb(id int64, gifThumb image.Image, claim mediablob.Claim) ([]*mtproto.PhotoSize, error) {
	if sizes, ok := c.reusedPhotoSizes(c.ctx, id, claim); ok {
		return sizes, nil
	}
	var (
		err    error
		szList = make([]*mtproto.PhotoSize, 0, 2)
	)

	isW := gifThumb.Bounds().Dx() >= gifThumb.Bounds().Dy()

	// 1. stripped
	stripped := bytes2.NewBuffer(make([]byte, 0, 4096))
	if isW {
		err = imaging.EncodeStripped(stripped, imaging.Resize(gifThumb, 40, 0), 30)
	} else {
		err = imaging.EncodeStripped(stripped, imaging.Resize(gifThumb, 0, 40), 30)
	}
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
		return nil, err
	}

	szList = append(szList, mtproto.MakeTLPhotoStrippedSize(&mtproto.PhotoSize{
		Type:  mtproto.PhotoSZStrippedType,
		Bytes: stripped.Bytes(),
	}).To_PhotoSize())

	// 2. photoSize
	thumb, szType := gifDisplayThumb(gifThumb)

	o := bytes2.NewBuffer(make([]byte, 0, 8192))
	err = imaging.EncodeJpeg(o, thumb)
	if err != nil {
		c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
		return nil, err
	}

	// 3. upload
	path := fmt.Sprintf("%s/%d.dat", szType, id)
	thumbSize := int64(len(o.Bytes()))
	if shared := c.originSizePaths(c.ctx, claim)[szType]; shared != "" {
		path = shared
	} else {
		info, err := c.svcCtx.Dao.PutPhotoFile(c.ctx, path, o.Bytes())
		if err != nil {
			c.Logger.Errorf("dfs.uploadGifDocumentMedia - error: %v", err)
			return nil, err
		}
		thumbSize = info.Size
	}
	c.rememberPhotoPath(c.ctx, id, szType, path)

	szList = append(szList, mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{
		Type:  szType,
		W:     int32(thumb.Bounds().Dx()),
		H:     int32(thumb.Bounds().Dy()),
		Size2: int32(thumbSize),
	}).To_PhotoSize())

	return szList, nil
}

// gifDisplayThumb is the JPEG preview written next to a GIF document.
// The size type matches the historical uploadGifThumb choice.
func gifDisplayThumb(img image.Image) (image.Image, string) {
	isW := img.Bounds().Dx() >= img.Bounds().Dy()
	sz := mtproto.GetMaxResizeInfo(false, img.Bounds().Dx(), img.Bounds().Dy())
	var thumb image.Image
	if isW {
		if img.Bounds().Dx() >= mtproto.PhotoSZMediumSize {
			thumb = imaging.Resize(img, mtproto.PhotoSZMediumSize, 0)
			sz.Type = mtproto.PhotoSZMediumType
		} else {
			thumb = imaging.Resize(img, 0, 320)
		}
	} else {
		if img.Bounds().Dy() >= mtproto.PhotoSZMediumSize {
			thumb = imaging.Resize(img, mtproto.PhotoSZMediumSize, 0)
			sz.Type = mtproto.PhotoSZMediumType
		} else {
			thumb = imaging.Resize(img, 0, sz.Size)
		}
	}
	return thumb, sz.Type
}

func photoSizeWH(sizes []*mtproto.PhotoSize) (int, int) {
	w, h := 0, 0
	for _, sz := range sizes {
		if sz.GetW() > 0 && sz.GetH() > 0 {
			w, h = int(sz.GetW()), int(sz.GetH())
		}
	}
	return w, h
}
