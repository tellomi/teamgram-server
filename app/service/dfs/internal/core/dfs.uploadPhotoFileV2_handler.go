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
	"crypto/md5"
	"fmt"
	"math/rand"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
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
		photoId    = c.nextID()
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
	subject := c.ledgerSubject(in.GetCreator())
	if noteErr := uploadledger.AllowClaim(subject, claim.Reused, claim.Size); noteErr != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - budget")
		return nil, mtproto.NewErrFloodWaitX(uploadledger.WaitOf(noteErr))
	}
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
		if err := uploadledger.CommitClaim(subject, claim.Reused, claim.Size); err != nil {
			c.Logger.Errorf("dfs.uploadPhotoFile - budget")
			return nil, mtproto.NewErrFloodWaitX(uploadledger.WaitOf(err))
		}
		releaseUploadHold(subject, file.GetId_INT64())
		return photo, nil
	}
	originPaths := c.originSizePaths(c.ctx, claim)

	type pendingSize struct {
		szType string
		w, h   int32
		bytes  []byte
		path   string
		size   int32
		puts   []sizePut
	}
	// Hot tier: every size key getFile can ask for is written locally
	// before this RPC returns. R2 is not on this path.
	hot := c.svcCtx.Dao != nil && c.svcCtx.Dao.MinioUtil != nil && c.svcCtx.Dao.MinioUtil.StoresHot(minio_util.ClassPhotos)
	var pending []pendingSize
	err = imaging.ReSizeImage(cacheData, ext, false, func(szType string, localId int, w, h int32, b []byte) error {
		// Hash hit keeps the first upload's key (ADR-0083 open question 1).
		proposed, write := photoSizeObject(originPaths, szType, photoId)
		record, puts := photoSizePuts(photoId, szType, proposed, b, cacheData, claim.Path, !write, hot)
		// Hot tier off: an aliased size is stored only as the original.
		// If that path cannot be recorded, write the conventional key so
		// download still has an object to open.
		if !hot && record != proposed {
			if c.savePhotoPath(c.ctx, photoId, szType, record) != nil {
				record = proposed
				puts = []sizePut{{path: proposed, body: b}}
			}
		}
		pending = append(pending, pendingSize{
			szType: szType,
			w:      w,
			h:      h,
			bytes:  b,
			path:   record,
			size:   int32(len(b)),
			puts:   puts,
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
	// advertised size is durable on the hot tier (or in R2 when that tier
	// is off). A client that calls upload.getFile for a size in photo.sizes
	// must not observe a missing object. None of these puts wait on R2
	// while the hot tier is on.
	puts := make([]func(context.Context) error, 0, len(pending)+1)
	if !claim.Reused {
		puts = append(puts, func(ctx context.Context) error {
			if err := c.svcCtx.Dao.EnsureLocal(ctx, minio_util.ClassPhotos, claim.Path, cacheData); err != nil {
				return err
			}
			fileSize.Size = int64(len(cacheData))
			return nil
		})
	}
	for i := range pending {
		for _, sp := range pending[i].puts {
			sp := sp
			puts = append(puts, func(ctx context.Context) error {
				return c.svcCtx.Dao.EnsureLocal(ctx, minio_util.ClassPhotos, sp.path, sp.body)
			})
		}
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
	if err := uploadledger.CommitClaim(subject, claim.Reused, claim.Size); err != nil {
		c.Logger.Errorf("dfs.uploadPhotoFile - budget")
		return nil, mtproto.NewErrFloodWaitX(uploadledger.WaitOf(err))
	}
	releaseUploadHold(subject, file.GetId_INT64())
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
