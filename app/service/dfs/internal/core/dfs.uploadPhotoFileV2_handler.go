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
	"context"
	"crypto/md5"
	"fmt"
	"math/rand"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
	"github.com/teamgram/teamgram-server/app/service/mediablob"
	"github.com/teamgram/teamgram-server/pkg/mediaotel"
	"github.com/teamgram/teamgram-server/pkg/uploadledger"

	"github.com/minio/minio-go/v7"
)

// DfsUploadPhotoFileV2
// dfs.uploadPhotoFileV2 creator:long file:InputFile = Photo;
func (c *DfsCore) DfsUploadPhotoFileV2(in *dfs.TLDfsUploadPhotoFileV2) (*mtproto.Photo, error) {
	var (
		fileSize  minio.UploadInfo
		cacheData []byte
		file      = in.GetFile()
		err       error
		r         *dao.SSDBReader
	)

	if file == nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - ErrInputRequestInvalid")
		return nil, mtproto.ErrInputRequestInvalid
	}

	if err = model.CheckFileParts(file.Parts); err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		return nil, err
	}

	r, err = c.svcCtx.Dao.OpenFile(c.ctx, in.GetCreator(), file.Id_INT64, file.Parts)
	if err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		return nil, partMissOr(err, mtproto.ErrMediaInvalid)
	}

	cacheData, err = r.ReadAll(c.ctx)
	if err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		return nil, partMissOr(err, mtproto.ErrMediaInvalid)
	} else {
		// log.Debugf("cacheData: %s", hex.EncodeToString(cacheData))
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

	claim, err := c.claimPhoto(c.ctx, photoId, cacheData)
	if err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		return nil, err
	}
	if claim.Reused && c.objectMissing(c.ctx, mediablob.ClassPhotos, claim.Path) {
		claim.Reused = false
	}
	uploadledger.NoteClaim(in.GetCreator(), claim.Reused, claim.Size)
	// One span for this upload: claim (hit or miss) and whether the
	// original object was written. Derivative sizes are not the blob.
	var objectWritten bool
	var bytesWritten int64
	var endUpload func(wrote bool, written int64)
	c.ctx, endUpload = mediaotel.BeginUpload(c.ctx, mediablob.ClassPhotos, claim.Reused, claim.Size)
	defer func() { endUpload(objectWritten, bytesWritten) }()
	photo := mtproto.MakeTLPhoto(&mtproto.Photo{
		HasStickers:   false,
		Id:            photoId,
		AccessHash:    accessHash,
		FileReference: []byte{},
		Date:          int32(r.Mtime),
		VideoSizes:    nil,
		DcId:          1,
	}).To_Photo()
	// Same bytes as an earlier photo: one object, this row's own access_hash.
	if sizes, ok := c.reusedPhotoSizes(c.ctx, photoId, claim); ok {
		photo.Sizes = sizes
		return photo, nil
	}
	originPaths := c.originSizePaths(c.ctx, claim)

	type pendingSize struct {
		szType string
		w, h   int32
		bytes  []byte
		path   string
		size   int32
		write  bool
	}
	var pending []pendingSize
	err = imaging.ReSizeImage(cacheData, ext, false, func(szType string, localId int, w, h int32, b []byte) error {
		// Hash hit keeps the first upload's key (ADR-0083 open question 1).
		path, write := photoSizeObject(originPaths, szType, photoId)
		// The largest step that was not resized is the original file.
		// Point it at the original object instead of putting those bytes
		// again. The path row is written before the RPC returns; if it
		// cannot be recorded, the conventional key is written so a client
		// that asks for this size type can still fetch it.
		if write && bytes.Equal(b, cacheData) && claim.Path != "" {
			if c.savePhotoPath(c.ctx, photoId, szType, claim.Path) == nil {
				path = claim.Path
				write = false
			}
		}
		pending = append(pending, pendingSize{
			szType: szType,
			w:      w,
			h:      h,
			bytes:  b,
			path:   path,
			size:   int32(len(b)),
			write:  write,
		})
		return nil
	})
	if err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		c.abandonClaim(c.ctx, mediablob.KindPhoto, photoId)
		return nil, mtproto.ErrImageProcessFailed
	}
	if len(pending) == 0 {
		c.Logger.Errorf("dfs.uploadPhotoFile - no sizes")
		c.abandonClaim(c.ctx, mediablob.KindPhoto, photoId)
		return nil, mtproto.ErrImageProcessFailed
	}

	// Original plus every derived object. The RPC returns only after each
	// advertised size is durable. A client that calls upload.getFile for a
	// size in photo.sizes must not observe a missing object.
	puts := make([]func(context.Context) error, 0, len(pending)+1)
	if !claim.Reused {
		puts = append(puts, func(ctx context.Context) error {
			info, err := c.svcCtx.Dao.PutPhotoFile(ctx, claim.Path, cacheData)
			if err != nil {
				return err
			}
			fileSize = info
			return nil
		})
	}
	for i := range pending {
		if !pending[i].write {
			continue
		}
		i := i
		body := pending[i].bytes
		path := pending[i].path
		puts = append(puts, func(ctx context.Context) error {
			info, err := c.svcCtx.Dao.PutPhotoFile(ctx, path, body)
			if err != nil {
				return err
			}
			if info.Size > 0 {
				pending[i].size = int32(info.Size)
			}
			return nil
		})
	}
	if err = runBounded(c.ctx, photoPutConcurrency, puts); err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - %v", err)
		c.abandonClaim(c.ctx, mediablob.KindPhoto, photoId)
		// Decode failures already returned IMAGE_PROCESS_FAILED. A put
		// error is the object store, and clients retry 500.
		return nil, mtproto.ErrInternalServerError
	}
	if !claim.Reused {
		objectWritten = true
		bytesWritten = int64(len(cacheData))
		if fileSize.Size > 0 {
			bytesWritten = fileSize.Size
		}
	}
	c.rememberPhotoPath(c.ctx, photoId, "0", claim.Path)
	for i := range pending {
		c.rememberPhotoPath(c.ctx, photoId, pending[i].szType, pending[i].path)
		sizeList = append(sizeList, mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{
			Type:  pending[i].szType,
			W:     pending[i].w,
			H:     pending[i].h,
			Size2: pending[i].size,
		}).To_PhotoSize())
	}

	photo.Sizes = sizeList
	return photo, nil
}

// savePhotoPath records the object key a size type is fetched from.
// A nil blob store cannot record it; the caller then writes the
// conventional <type>/<id>.dat key, which is what download falls back to.
func (c *DfsCore) savePhotoPath(ctx context.Context, photoID int64, sizeType, path string) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Blobs == nil {
		return fmt.Errorf("dfs: photo path store is not configured")
	}
	if err := c.svcCtx.Dao.Blobs.SavePhotoPath(ctx, photoID, sizeType, path); err != nil {
		c.Logger.Errorf("dfs.savePhotoPath %s - %v", sizeType, err)
		return err
	}
	return nil
}
