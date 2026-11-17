// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/files/cdn"
	"github.com/teamgram/teamgram-server/app/bff/helpdesk"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/media/retention"
	"github.com/teamgram/teamgram-server/pkg/msgexpiry"
)

// downloadFile is set by tests. Production uses DFS.
var downloadFile func(ctx context.Context, location *mtproto.InputFileLocation, offset int64, limit int32) (*mtproto.Upload_File, error)

// UploadGetFile
// upload.getFile#be5335be flags:# precise:flags.0?true cdn_supported:flags.1?true location:InputFileLocation offset:long limit:int = upload.File;
// https://core.telegram.org/method/upload.getFile
// https://core.telegram.org/cdn
//
// When TELLOMI_CDN_PUBLIC_KEYS is empty, the bytes come from the master DFS
// as before. When keys are published and the client set cdn_supported, a
// public file that fits in the CDN cache is returned as upload.fileCdnRedirect
// with a file_token the getCdnFile / reuploadCdnFile cache already accepts.
func (c *FilesCore) UploadGetFile(in *mtproto.TLUploadGetFile) (*mtproto.Upload_File, error) {
	var (
		location = in.GetLocation()
		offset   = in.GetOffset_INT64()
		limit    = in.GetLimit()
	)

	if offset == 0 {
		offset = int64(in.GetOffset_INT32())
	}

	switch location.GetPredicateName() {
	case mtproto.Predicate_inputFileLocation:
		// inputFileLocation#dfdaabe1
		//	volume_id:long
		//	local_id:int
		//	secret:long
		//	file_reference:bytes = InputFileLocation;

		err := mtproto.ErrInputRequestInvalid
		c.Logger.Errorf("upload.getFile - error: %v inputFileLocation", err)
		return nil, err
	case mtproto.Predicate_inputEncryptedFileLocation:
		// inputEncryptedFileLocation#f5235d55
		//	id:long
		//	access_hash:long = InputFileLocation;

	case mtproto.Predicate_inputDocumentFileLocation:
		// inputDocumentFileLocation#bad07584
		//	id:long
		//	access_hash:long
		//	file_reference:bytes
		//	thumb_size:string = InputFileLocation;
		//
	case mtproto.Predicate_inputSecureFileLocation:
		// inputSecureFileLocation#cbc7ee28
		//	id:long
		//	access_hash:long = InputFileLocation;
		//
	case mtproto.Predicate_inputTakeoutFileLocation:
		// inputTakeoutFileLocation#29be5899 = InputFileLocation;
		//
	case mtproto.Predicate_inputPhotoFileLocation, mtproto.Predicate_inputPhotoLegacyFileLocation:
		// inputPhotoFileLocation#40181ffe
		//	id:long
		//	access_hash:long
		//	file_reference:bytes
		//	thumb_size:string = InputFileLocation;
		// inputPhotoLegacyFileLocation carries the same id and access_hash.
		//
	case mtproto.Predicate_inputPeerPhotoFileLocation:
		// inputPeerPhotoFileLocation#37257e99 flags:#
		//	big:flags.0?true
		//	peer:InputPeer
		//	photo_id:long = InputFileLocation;
		//
	case mtproto.Predicate_inputStickerSetThumb:
		// inputStickerSetThumb#9d84f3db
		//	stickerset:InputStickerSet
		//	thumb_version:int = InputFileLocation;
		location2, err := c.stickerSetThumb(location)
		if err != nil {
			c.Logger.Errorf("upload.getFile - error: %v inputStickerSetThumb", err)
			return nil, err
		}
		location = location2
	case mtproto.Predicate_inputGroupCallStream:
		// inputGroupCallStream#bba51639
		//	call:InputGroupCall
		//	time_ms:long
		//	scale:int = InputFileLocation;
		//
		if c.svcCtx.Plugin2 != nil {
			uploadFile, err := c.svcCtx.Plugin2.GetGroupCallStreamFile(
				c.ctx,
				c.MD.UserId,
				in.GetLocation())
			if err != nil {
				c.Logger.Errorf("upload.getFile - error: %v inputFileLocation", err)
				return nil, err
			}
			return uploadFile, nil
		}
	default:
		c.Logger.Errorf("upload.getFile - error: invalid location")
		return nil, mtproto.ErrLocationInvalid
	}

	if err := c.authorize(location); err != nil {
		c.Logger.Errorf("upload.getFile - access: %v", err)
		return nil, err
	}

	if msgexpiry.Default().BlocksDownload(location.GetId(), int32(time.Now().Unix())) || c.mediaExpired(location) {
		// FILE_ID_INVALID is what clients already handle for a file that is
		// gone: they stop the download and keep the thumbnail. FILE_REFERENCE_*
		// would make them refetch the message and retry forever.
		c.Logger.Infof("upload.getFile - %s %d expired by media retention", location.GetPredicateName(), location.GetId())
		return nil, mtproto.ErrFileIdInvalid
	}

	if c.wantsCDN(in) {
		if full, ok := fullSeeded(location); ok {
			if redir, ok := c.cdnRedirect(location, full); ok {
				return redir, nil
			}
		}
	}
	if file, ok := seededDocumentFile(location, offset, limit); ok {
		return file, nil
	}

	if c.wantsCDN(in) && cdnLocation(location) {
		if redir, ok := c.cachedRedirect(location); ok {
			return redir, nil
		}
		probe, err := c.download(location, 0, cdn.MaxPlain+1)
		if err == nil && probe != nil {
			body := probe.GetBytes()
			if len(body) > 0 && len(body) <= cdn.MaxPlain {
				if redir, ok := c.cdnRedirect(location, body); ok {
					return redir, nil
				}
			}
		}
	}

	if location.GetPredicateName() == mtproto.Predicate_inputPhotoFileLocation {
		if body, mime, ok := chatbox.WebPhotoForDownload(location.GetId(), location.GetAccessHash()); ok {
			return uploadFileChunk(sliceFile(body, offset, limit), mime), nil
		}
	}

	uploadFile, err := c.download(location, offset, limit)
	if err != nil {
		c.Logger.Errorf("upload.getFile - error: %v", err.Error())
		return nil, err
	}
	if uploadFile.GetType().GetPredicateName() == mtproto.Predicate_storage_fileUnknown {
		uploadFile.Type.PredicateName = mtproto.Predicate_storage_filePartial
	}

	return uploadFile, nil
}

// stickerSetThumb turns inputStickerSetThumb into the cover document.
// The sticker catalog is the source of that cover. A process that did not
// inject the optional plugin still serves it; it does not return
// ENTERPRISE_IS_BLOCKED.
func (c *FilesCore) stickerSetThumb(location *mtproto.InputFileLocation) (*mtproto.InputFileLocation, error) {
	var (
		loc *mtproto.InputFileLocation
		err error
	)
	if c != nil && c.svcCtx != nil && c.svcCtx.Plugin1 != nil && c.MD != nil {
		loc, err = c.svcCtx.Plugin1.GetStickerSetThumbFileLocation(c.ctx, c.MD.UserId, location.GetStickerset(), location.GetThumbVersion())
	} else {
		loc, err = catalog.Must().ThumbLocation(location.GetStickerset())
	}
	if err != nil {
		return nil, err
	}
	if loc == nil {
		return nil, mtproto.ErrStickerIdInvalid
	}
	return loc, nil
}

// mediaExpired reports whether location names an object the retention
// sweeper removed. Thumbnails stay downloadable.
func (c *FilesCore) mediaExpired(location *mtproto.InputFileLocation) bool {
	if c.svcCtx == nil || c.svcCtx.Retention == nil {
		return false
	}
	kind, ok := expiredKind(location)
	if !ok {
		return false
	}
	return c.svcCtx.Retention.Expired(c.ctx, kind, location.GetId())
}

func expiredKind(location *mtproto.InputFileLocation) (retention.Kind, bool) {
	switch location.GetPredicateName() {
	case mtproto.Predicate_inputDocumentFileLocation:
		return retention.KindDocument, location.GetThumbSize() == ""
	case mtproto.Predicate_inputPhotoFileLocation:
		return retention.KindPhoto, retention.FullPhotoSize(location.GetThumbSize())
	case mtproto.Predicate_inputEncryptedFileLocation:
		return retention.KindEncrypted, true
	}
	return "", false
}

func (c *FilesCore) wantsCDN(in *mtproto.TLUploadGetFile) bool {
	return in.GetCdnSupported() && len(helpdesk.PublicKeys()) > 0
}

func cdnLocation(location *mtproto.InputFileLocation) bool {
	switch location.GetPredicateName() {
	case mtproto.Predicate_inputDocumentFileLocation,
		mtproto.Predicate_inputPhotoFileLocation,
		mtproto.Predicate_inputStickerSetThumb:
		return true
	default:
		return false
	}
}

func (c *FilesCore) cdnRedirect(location *mtproto.InputFileLocation, plain []byte) (*mtproto.Upload_File, bool) {
	keys := helpdesk.PublicKeys()
	if len(keys) == 0 {
		return nil, false
	}
	issued, err := cdn.Shared().Issue(cdn.LocationKey(location), plain)
	if err != nil || issued == nil || len(issued.FileToken) == 0 {
		c.Logger.Errorf("upload.getFile - cdn stage: %v", err)
		return nil, false
	}
	return redirectFile(keys[0].DC, issued), true
}

func (c *FilesCore) cachedRedirect(location *mtproto.InputFileLocation) (*mtproto.Upload_File, bool) {
	keys := helpdesk.PublicKeys()
	if len(keys) == 0 {
		return nil, false
	}
	issued, ok := cdn.Shared().Lookup(cdn.LocationKey(location))
	if !ok {
		return nil, false
	}
	return redirectFile(keys[0].DC, issued), true
}

func redirectFile(dc int32, issued *cdn.Issued) *mtproto.Upload_File {
	return mtproto.MakeTLUploadFileCdnRedirect(&mtproto.Upload_File{
		DcId:          dc,
		FileToken:     issued.FileToken,
		EncryptionKey: issued.Key,
		EncryptionIv:  issued.IV,
		FileHashes:    issued.Hashes,
	}).To_Upload_File()
}

func (c *FilesCore) download(location *mtproto.InputFileLocation, offset int64, limit int32) (*mtproto.Upload_File, error) {
	if downloadFile != nil {
		return downloadFile(c.ctx, location, offset, limit)
	}
	return c.svcCtx.Dao.DfsClient.DfsDownloadFile(c.ctx, &dfs.TLDfsDownloadFile{
		Location: location,
		Offset:   offset,
		Limit:    limit,
	})
}
