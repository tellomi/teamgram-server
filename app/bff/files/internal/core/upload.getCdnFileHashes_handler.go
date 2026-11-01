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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/files/cdn"
	"github.com/teamgram/teamgram-server/app/bff/helpdesk"
)

// UploadGetCdnFileHashes
// upload.getCdnFileHashes#91dc3f31 file_token:bytes offset:long = Vector<FileHash>;
// https://core.telegram.org/method/upload.getCdnFileHashes
//
// Master DC method. Returns SHA-256 hashes of ciphertext parts at or after
// offset. An unpublished CDN (empty help.getCdnConfig) has no tokens.
func (c *FilesCore) UploadGetCdnFileHashes(in *mtproto.TLUploadGetCdnFileHashes) (*mtproto.Vector_FileHash, error) {
	offset := cdnOffset(in.GetOffset_INT64(), in.GetOffset_INT32())
	if len(helpdesk.PublicKeys()) == 0 {
		c.Logger.Errorf("upload.getCdnFileHashes - error: CDN is not published")
		return nil, mtproto.ErrFileTokenInvalid
	}
	hashes, err := cdn.Shared().Hashes(in.GetFileToken(), offset)
	if err != nil {
		c.Logger.Errorf("upload.getCdnFileHashes - error: %v", err)
		return nil, err
	}
	return &mtproto.Vector_FileHash{Datas: hashes}, nil
}
