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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/location"

	"google.golang.org/grpc/status"
)

var (
	ErrWebfileNotAvailable = status.Error(mtproto.ErrBadRequest, "WEBFILE_NOT_AVAILABLE")
)

// UploadGetWebFile
// upload.getWebFile#24e6818d location:InputWebFileLocation offset:int limit:int = upload.WebFile;
func (c *FilesCore) UploadGetWebFile(in *mtproto.TLUploadGetWebFile) (*mtproto.Upload_WebFile, error) {
	if in == nil || in.GetLocation() == nil {
		return nil, ErrWebfileNotAvailable
	}
	switch in.GetLocation().GetPredicateName() {
	case mtproto.Predicate_inputWebFileGeoPointLocation:
		return c.webFileGeo(in)
	case mtproto.Predicate_inputWebFileAudioAlbumThumbLocation:
		err := ErrWebfileNotAvailable
		c.Logger.Errorf("upload.getWebFile - error: %v", err)

		return nil, err
	default:
		c.Logger.Errorf("upload.getWebFile blocked, License key from https://teamgram.net required to unlock enterprise features.")

		return nil, mtproto.ErrEnterpriseIsBlocked
	}
}

func (c *FilesCore) webFileGeo(in *mtproto.TLUploadGetWebFile) (*mtproto.Upload_WebFile, error) {
	loc := in.GetLocation()
	gp := loc.GetGeoPoint()
	if gp == nil || gp.GetPredicateName() == mtproto.Predicate_inputGeoPointEmpty {
		return nil, ErrWebfileNotAvailable
	}
	var userID int64
	if c.MD != nil {
		userID = c.MD.GetUserId()
	}
	body, ok := location.StaticMap(c.ctx, userID, gp.GetLat(), gp.GetLong(), loc.GetW(), loc.GetH(), loc.GetZoom(), loc.GetScale())
	if !ok || len(body) == 0 {
		// TELLOMI_AMAP_KEY unset, or the map service failed. This is the
		// official error. Do not substitute ENTERPRISE_IS_BLOCKED.
		return nil, ErrWebfileNotAvailable
	}
	chunk := sliceWebFile(body, in.GetOffset(), in.GetLimit())
	return mtproto.MakeTLUploadWebFile(&mtproto.Upload_WebFile{
		Size2:    int32(len(body)),
		MimeType: "image/png",
		FileType: mtproto.MakeTLStorageFilePng(nil).To_Storage_FileType(),
		Mtime:    int32(time.Now().Unix()),
		Bytes:    chunk,
	}).To_Upload_WebFile(), nil
}

func sliceWebFile(body []byte, offset, limit int32) []byte {
	if offset < 0 {
		offset = 0
	}
	if int(offset) >= len(body) {
		return []byte{}
	}
	rest := body[offset:]
	if limit <= 0 || int(limit) > len(rest) {
		return rest
	}
	return rest[:limit]
}
