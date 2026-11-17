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
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
	"golang.org/x/sync/singleflight"
)

// downloadFlight coalesces identical in-flight getFile reads. The chunk
// cache does this again for a cacheable class; this also covers the
// catalog routing and a thumb rebuild that has not been stored yet.
// The key is the object, not the access_hash. upload.getFile checks that
// hash before it calls dfs.
var downloadFlight singleflight.Group

// DfsDownloadFile
// dfs.downloadFile location:InputFileLocation offset:int limit:int = upload.File;
func (c *DfsCore) DfsDownloadFile(in *dfs.TLDfsDownloadFile) (*mtproto.Upload_File, error) {
	v, err, _ := downloadFlight.Do(downloadFlightKey(in), func() (any, error) {
		return c.downloadFileBody(in)
	})
	if err != nil || v == nil {
		return nil, err
	}
	return cloneUploadFile(v.(*mtproto.Upload_File)), nil
}

func downloadFlightKey(in *dfs.TLDfsDownloadFile) string {
	loc := in.GetLocation()
	if loc == nil {
		return "nil"
	}
	return fmt.Sprintf("%s\x00%d\x00%d\x00%s\x00%d\x00%d",
		loc.GetPredicateName(), loc.GetId(), loc.GetPhotoId(), loc.GetThumbSize(), in.GetOffset(), in.GetLimit())
}

func cloneUploadFile(in *mtproto.Upload_File) *mtproto.Upload_File {
	if in == nil {
		return nil
	}
	out := *in
	if in.Bytes != nil {
		out.Bytes = append([]byte(nil), in.Bytes...)
	}
	return &out
}

func (c *DfsCore) downloadFileBody(in *dfs.TLDfsDownloadFile) (*mtproto.Upload_File, error) {
	var (
		bytes []byte
		sType int32

		err error

		location = in.GetLocation()
		offset   = in.GetOffset()
		limit    = in.GetLimit()
	)

	switch location.GetPredicateName() {
	case mtproto.Predicate_inputFileLocation:
		//inputFileLocation#dfdaabe1 volume_id:long local_id:int secret:long file_reference:bytes = InputFileLocation;

		err = mtproto.ErrInputRequestInvalid
		c.Logger.Errorf("dfs.downloadFile - error: %v", err)
		return nil, err

	case mtproto.Predicate_inputEncryptedFileLocation:
		//inputEncryptedFileLocation#f5235d55
		//	id:long
		//	access_hash:long = InputFileLocation;

		bytes, err = c.svcCtx.Dao.GetCacheFile(c.ctx, location.GetId(), offset, limit, fmt.Sprintf("%d.dat", location.GetId()), c.svcCtx.Dao.MinioUtil.GetEncryptedFile)
		if err != nil {
			c.Logger.Errorf("download file: %v", err)
			err = nil
			bytes = []byte{}
		}
		sType = int32(mtproto.CRC32_storage_filePartial)
	case mtproto.Predicate_inputDocumentFileLocation:
		//inputDocumentFileLocation#bad07584
		//	id:long
		//	access_hash:long
		//	file_reference:bytes
		//	thumb_size:string = InputFileLocation;

		if location.GetThumbSize() == "" {
			// Official sticker and emoji documents live in the public
			// bucket. Reading documents/ first is an R2 miss on every
			// download. User files stay on the documents path.
			if class, key, ok := catalog.PublicObject(location.GetId()); ok {
				bytes, err = c.readPublicDocument(class, key, offset, limit)
				if errors.Is(err, io.EOF) {
					// A read past the end is a short success. An empty
					// object at offset 0 is a miss.
					if offset > 0 || len(bytes) > 0 {
						if bytes == nil {
							bytes = []byte{}
						}
						err = nil
					} else {
						err = minio_util.ErrNotFound
					}
				}
				if err != nil {
					c.Logger.Errorf("download catalog %s/%s: %v", class, key, err)
					if errors.Is(err, minio_util.ErrNotFound) {
						return nil, mtproto.ErrFileIdInvalid
					}
					return nil, err
				}
			} else {
				path := c.svcCtx.Dao.DocumentObjectPath(c.ctx, location.GetId())
				bytes, err = c.svcCtx.Dao.GetCacheFile(c.ctx, location.GetId(), offset, limit, path, c.svcCtx.Dao.MinioUtil.GetDocumentFile)
				if err != nil {
					// A catalog id the index has not loaded yet, or a
					// wallpaper stored only on the public prefix.
					bytes, err = readFirst(
						func() ([]byte, error) {
							return c.svcCtx.Dao.GetStickerFile(c.ctx, path, offset, limit)
						},
						func() ([]byte, error) {
							return c.svcCtx.Dao.GetEmojiFile(c.ctx, path, offset, limit)
						},
						func() ([]byte, error) {
							return c.svcCtx.Dao.GetDocumentFile(c.ctx, path, offset, limit)
						},
						func() ([]byte, error) {
							return c.svcCtx.Dao.GetWallpaperFile(c.ctx, path, offset, limit)
						},
					)
					if err != nil {
						c.Logger.Errorf("download file: %v", err)
						if errors.Is(err, minio_util.ErrNotFound) {
							return nil, mtproto.ErrFileIdInvalid
						}
						return nil, err
					}
				}
			}
			sType = int32(location.GetAccessHash() >> 32)
		} else {
			paths := c.documentThumbPaths(location.GetId(), location.GetThumbSize())
			isVideo := mtproto.PhotoSizeIsVideo(location.GetThumbSize())
			c.Logger.Infof("document thumb candidates: %d", len(paths))
			if isVideo {
				bytes, err = c.readDocumentThumb(offset, limit, paths, c.svcCtx.Dao.GetVideoFile, c.svcCtx.Dao.GetAvatarFile)
				sType = int32(mtproto.CRC32_storage_fileMp4)
			} else {
				bytes, err = c.readDocumentThumb(offset, limit, paths, c.svcCtx.Dao.GetPhotoFile, c.svcCtx.Dao.GetWallpaperFile)
				sType = int32(mtproto.CRC32_storage_fileJpeg)
			}
			bytes, err = c.finishDocumentThumb(location, offset, limit, bytes, err)
			if err != nil {
				return nil, err
			}
		}
	case mtproto.Predicate_inputSecureFileLocation:
		//inputSecureFileLocation#cbc7ee28 id:long access_hash:long = InputFileLocation;

		err = mtproto.ErrInputRequestInvalid
		c.Logger.Errorf("dfs.downloadFile - error: %v", err)
		return nil, err
	case mtproto.Predicate_inputTakeoutFileLocation:
		//inputTakeoutFileLocation#29be5899 = InputFileLocation;

		err = mtproto.ErrInputRequestInvalid
		c.Logger.Errorf("dfs.downloadFile - error: %v", err)
		return nil, err
	case mtproto.Predicate_inputPhotoLegacyFileLocation:
		//inputPhotoLegacyFileLocation#d83466f3 id:long access_hash:long file_reference:bytes volume_id:long local_id:int secret:long = InputFileLocation;

		err = mtproto.ErrInputRequestInvalid
		c.Logger.Errorf("dfs.downloadFile - error: %v", err)
		return nil, err
	case mtproto.Predicate_inputPhotoFileLocation:
		//inputPhotoFileLocation#40181ffe id:long access_hash:long file_reference:bytes thumb_size:string = InputFileLocation;

		isVideo := mtproto.PhotoSizeIsVideo(location.GetThumbSize())
		if isVideo {
			bytes, err = c.readStoredPhoto(location.GetId(), location.GetThumbSize(), offset, limit, func(path string) ([]byte, error) {
				return c.svcCtx.Dao.GetVideoFile(c.ctx, path, offset, limit)
			})
			sType = int32(mtproto.CRC32_storage_fileMp4)
		} else {
			bytes, err = c.readStoredPhoto(location.GetId(), location.GetThumbSize(), offset, limit, func(path string) ([]byte, error) {
				return c.svcCtx.Dao.GetPhotoFile(c.ctx, path, offset, limit)
			})
			sType = int32(location.GetAccessHash() >> 32)
		}
		if err != nil {
			// log.Warnf("download file: %v", err)
			err = nil
			bytes = []byte{}
		}
	case mtproto.Predicate_inputPeerPhotoFileLocation:
		//inputPeerPhotoFileLocation#37257e99 flags:# big:flags.0?true peer:InputPeer photo_id:long = InputFileLocation;
		// big is the 640px profile (c). Otherwise the 160px list avatar (a).
		// A missing size falls through to the next ladder step. An empty
		// upload.file is a successful download to the client, which then
		// caches the placeholder.

		bytes, err = c.readPeerPhoto(location.GetPhotoId(), location.GetBig(), offset, limit)
		if err != nil {
			c.Logger.Infof("download file: %v", err)
			return nil, err
		}
		sType = int32(mtproto.CRC32_storage_fileJpeg)
	case mtproto.Predicate_inputStickerSetThumb:
		//inputStickerSetThumb#9d84f3db stickerset:InputStickerSet thumb_version:int = InputFileLocation;

		path := fmt.Sprintf("m/%d.dat", location.GetId())
		c.Logger.Infof("path: %s", path)
		bytes, err = readFirst(
			func() ([]byte, error) {
				return c.svcCtx.Dao.GetStickerFile(c.ctx, path, offset, limit)
			},
			func() ([]byte, error) {
				return c.svcCtx.Dao.GetPhotoFile(c.ctx, path, offset, limit)
			},
		)
		if err != nil {
			c.Logger.Infof("download file: %v", err)
			err = nil
			bytes = []byte{}
		}
		// sType = int32(location.GetAccessHash() >> 32)
		sType = int32(mtproto.CRC32_storage_fileJpeg)
	case mtproto.Predicate_inputGroupCallStream:
		//inputGroupCallStream#bba51639 call:InputGroupCall time_ms:long scale:int = InputFileLocation;

		err = mtproto.ErrInputRequestInvalid
		c.Logger.Errorf("dfs.downloadFile - error: %v", err)
		return nil, err
	default:
		err = mtproto.ErrInputRequestInvalid
		c.Logger.Errorf("dfs.downloadFile - error: %v", err)
		return nil, err
	}

	uploadFile := mtproto.MakeTLUploadFile(&mtproto.Upload_File{
		Type:  model.MakeStorageFileType(sType),
		Mtime: int32(time.Now().Unix()),
		Bytes: bytes,
	}).To_Upload_File()

	return uploadFile, nil
}
