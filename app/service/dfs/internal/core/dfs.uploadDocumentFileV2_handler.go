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
	"io"
	"math/rand"

	"github.com/teamgram/marmota/pkg/bytes2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
	"github.com/teamgram/teamgram-server/app/service/mediablob"
	"github.com/teamgram/teamgram-server/pkg/mediaotel"
	"github.com/teamgram/teamgram-server/pkg/uploadledger"
)

// DfsUploadDocumentFileV2
// dfs.uploadDocumentFileV2 creator:long media:InputMedia = Document;
func (c *DfsCore) DfsUploadDocumentFileV2(in *dfs.TLDfsUploadDocumentFileV2) (*mtproto.Document, error) {
	var (
		documentId = c.nextID()
		//idgen.GetUUID()
		file      = in.GetMedia().GetFile()
		cacheData []byte
	)

	// 有点难理解，主要是为了不在这里引入snowflake
	ext := model.GetFileExtName(file.GetName())
	extType := model.GetStorageFileTypeConstructor(ext)
	accessHash := int64(extType)<<32 | int64(rand.Uint32())

	r, err := c.svcCtx.Dao.OpenFile(c.ctx, in.GetCreator(), file.Id_INT64, file.Parts)
	if err != nil {
		c.Logger.Errorf("dfs.uploadDocumentFile - %v", err)
		return nil, partMissOr(err, mtproto.ErrMediaInvalid)
	}

	//fileInfo, err := s.Dao.GetFileInfo(ctx, creatorId, file.Id)
	//if err != nil {
	//	log.Errorf("dfs.uploadDocumentFile - error: %v", err)
	//	return nil, err
	//}

	// Set after the claim: a reused document must download the existing
	// object, not this upload's temporary parts.

	//go func() {
	//	_, err2 := s.Dao.PutDocumentFile(ctx,
	//		fmt.Sprintf("%d.dat", documentId),
	//		s.Dao.NewSSDBReader(r.DfsFileInfo))
	//	if err2 != nil {
	//		log.Errorf("dfs.uploadDocumentFile - error: %v", err2)
	//	}
	//}()

	attributes := make([]*mtproto.DocumentAttribute, 0, len(in.GetMedia().Attributes))
	for _, attr := range in.GetMedia().GetAttributes() {
		switch attr.GetPredicateName() {
		case mtproto.Predicate_documentAttributeAnimated:
		case mtproto.Predicate_documentAttributeFilename:
			if attr.GetFileName() != "" {
				attributes = append(attributes, attr)
			}
		case mtproto.Predicate_documentAttributeAudio:
			if in.GetMedia().GetMimeType() == "audio/ogg" {
				if attr.Voice == true {
					attributes = append(attributes, attr)
				}
			} else {
				attributes = append(attributes, attr)
			}
		default:
			attributes = append(attributes, attr)
		}
	}

	// document#1e87342b flags:#
	//	id:long
	//	access_hash:long
	//	file_reference:bytes
	//	date:int
	//	mime_type:string
	//	size:int
	//	thumbs:flags.0?Vector<PhotoSize>
	//	video_thumbs:flags.1?Vector<VideoSize>
	//	dc_id:int
	//	attributes:Vector<DocumentAttribute> = Document;
	document := mtproto.MakeTLDocument(&mtproto.Document{
		Id:            documentId,
		AccessHash:    accessHash,
		FileReference: nil,
		Date:          int32(r.DfsFileInfo.Mtime),
		MimeType:      in.GetMedia().GetMimeType(),
		Size2:         r.DfsFileInfo.GetFileSize(),
		Size2_INT32:   int32(r.DfsFileInfo.GetFileSize()),
		Size2_INT64:   r.DfsFileInfo.GetFileSize(),
		Thumbs:        nil,
		VideoThumbs:   nil,
		DcId:          1,
		Attributes:    attributes,
	}).To_Document()

	isThumb := mtproto.IsMimeAcceptedForPhotoVideoAlbum(document.MimeType) && model.IsFileExtImage(ext)
	var claim mediablob.Claim
	var finishUpload func(bool, int64)
	uploadScheduled := false
	defer func() {
		if finishUpload != nil && !uploadScheduled {
			finishUpload(false, 0)
		}
	}()
	if isThumb {
		var (
			thumb image.Image
			// photoId = idgen.GetUUID()
			// ext2     = ".jpg"
			// extType2 = model.GetStorageFileTypeConstructor(ext2)
			// secretId = int64(extType2)<<32 | int64(rand.Uint32())
		)

		cacheData, err = r.ReadAll(c.ctx)
		if err != nil {
			c.Logger.Errorf("dfs.uploadDocumentFile - %v", err)
			return nil, partMissOr(err, mtproto.ErrWallpaperFileInvalid)
		} else {
			// log.Debugf("cacheData: %s", hex.EncodeToString(cacheData))
		}
		claim = c.claimDocumentBytes(c.ctx, documentId, cacheData)
		c.ctx, finishUpload = mediaotel.BeginUpload(c.ctx, mediablob.ClassDocuments, claim.Reused, claim.Size)
		if sizes, ok := c.reusedPhotoSizes(c.ctx, documentId, claim); ok {
			document.Thumbs = sizes
		} else if thumb, err = imaging.Decode(bytes.NewReader(cacheData)); err != nil {
			// ioutil.WriteFile("./t.jpg", cacheData, 0644)
			c.Logger.Errorf("dfs.uploadDocumentFile - error: %v", err)
			// The body is still these bytes. A bad image just has no thumb.
		} else {
			// build photoStrippedSize
			stripped := bytes2.NewBuffer(make([]byte, 0, 4096))
			if thumb.Bounds().Dx() >= thumb.Bounds().Dy() {
				err = imaging.EncodeStripped(stripped, imaging.Resize(thumb, 40, 0), 30)
			} else {
				err = imaging.EncodeStripped(stripped, imaging.Resize(thumb, 0, 40), 30)
			}
			if err != nil {
				c.Logger.Errorf("dfs.uploadDocumentFile - error: %v", err)
				c.abandonClaim(c.ctx, mediablob.KindDocument, documentId)
				return nil, err
			}

			// upload thumb
			var (
				mThumbData = bytes2.NewBuffer(make([]byte, 0, len(cacheData)))
				mThumb     image.Image
			)
			if thumb.Bounds().Dx() >= thumb.Bounds().Dy() {
				mThumb = imaging.Resize(thumb, 320, 0)
				// err = imaging.Encode(mThumbData, mThumb, 80)
			} else {
				mThumb = imaging.Resize(thumb, 0, 320)
				// err = imaging.Encode(mThumbData, imaging.Resize(thumb, 0, 320), 80)
			}

			err = imaging.EncodeJpeg(mThumbData, mThumb)
			if err != nil {
				c.Logger.Errorf("dfs.uploadDocumentFile - error: %v", err)
				c.abandonClaim(c.ctx, mediablob.KindDocument, documentId)
				return nil, err
			}

			// upload thumb
			path := fmt.Sprintf("%s/%d.dat", mtproto.PhotoSZMediumType, documentId)
			if shared := c.originSizePaths(c.ctx, claim)[mtproto.PhotoSZMediumType]; shared != "" {
				path = shared
			} else if _, err = c.svcCtx.Dao.PutPhotoFile(c.ctx, path, mThumbData.Bytes()); err != nil {
				// A photo_sizes row for an object that was never stored makes
				// clients retry the empty download forever.
				c.Logger.Errorf("dfs.uploadDocumentFile - thumb put: %v", err)
				c.abandonClaim(c.ctx, mediablob.KindDocument, documentId)
				return nil, err
			}
			c.rememberPhotoPath(c.ctx, documentId, mtproto.PhotoSZMediumType, path)

			document.Thumbs = []*mtproto.PhotoSize{
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
	} else {
		var err2 error
		claim, err2 = c.claimDocumentReader(c.ctx, documentId, c.svcCtx.Dao.NewSSDBReader(r.DfsFileInfo))
		if err2 != nil {
			c.Logger.Errorf("dfs.uploadDocumentFile - %v", err2)
			return nil, partMissOr(err2, mtproto.ErrMediaInvalid)
		}
		c.ctx, finishUpload = mediaotel.BeginUpload(c.ctx, mediablob.ClassDocuments, claim.Reused, claim.Size)
	}
	applyDocumentSize(document, claim.Size)
	if claim.Reused && c.objectMissing(c.ctx, mediablob.ClassDocuments, claim.Path) {
		claim.Reused = false
	}
	subject := c.ledgerSubject(in.GetCreator())
	if noteErr := uploadledger.AllowClaim(subject, claim.Reused, claim.Size); noteErr != nil {
		c.Logger.Errorf("dfs.uploadDocumentFile - budget")
		return nil, mtproto.NewErrFloodWaitX(uploadledger.WaitOf(noteErr))
	}
	if !claim.Reused {
		c.svcCtx.Dao.SetCacheFileInfo(c.ctx, documentId, r.DfsFileInfo)
	}

	// The object has to be durable before the RPC returns. A 5xx from the
	// store is INTERNAL_SERVER_ERROR, and the claim is dropped so the same
	// bytes can be uploaded again.
	if !claim.Reused {
		var body io.Reader
		if len(cacheData) > 0 {
			body = bytes.NewReader(cacheData)
		} else {
			body = c.svcCtx.Dao.NewSSDBReader(r.DfsFileInfo)
		}
		info, err2 := c.svcCtx.Dao.PutDocumentFile(c.ctx, claim.Path, body)
		if err2 != nil {
			c.Logger.Errorf("dfs.uploadDocumentFile - error: %v", err2)
			c.abandonClaim(c.ctx, mediablob.KindDocument, documentId)
			if finishUpload != nil {
				finishUpload(false, 0)
				uploadScheduled = true
			}
			return nil, mtproto.ErrInternalServerError
		}
		if finishUpload != nil {
			written := claim.Size
			if info.Size > 0 {
				written = info.Size
			}
			finishUpload(true, written)
			uploadScheduled = true
		}
	} else if finishUpload != nil {
		finishUpload(false, 0)
		uploadScheduled = true
	}
	if err := uploadledger.CommitClaim(subject, claim.Reused, claim.Size); err != nil {
		c.Logger.Errorf("dfs.uploadDocumentFile - budget")
		return nil, mtproto.NewErrFloodWaitX(uploadledger.WaitOf(err))
	}
	releaseUploadHold(subject, file.GetId_INT64())
	return document, nil
}

func releaseUploadHold(user, fileID int64) {
	if led := uploadledger.Shared(); led != nil {
		led.ReleaseFile(user, fileID)
	}
}
