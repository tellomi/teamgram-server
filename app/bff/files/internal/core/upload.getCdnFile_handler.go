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

// UploadGetCdnFile
// upload.getCdnFile#395f69da file_token:bytes offset:long limit:int = upload.CdnFile;
// https://core.telegram.org/method/upload.getCdnFile
// https://core.telegram.org/cdn
//
// CDN DCs answer this. This process is the master DC. With no
// TELLOMI_CDN_PUBLIC_KEYS it is not a CDN, so every token is
// FILE_TOKEN_INVALID and the client continues with upload.getFile.
// A published CDN returns cdnFileReuploadNeeded until reuploadCdnFile,
// then the AES-256-CTR ciphertext for that range.
func (c *FilesCore) UploadGetCdnFile(in *mtproto.TLUploadGetCdnFile) (*mtproto.Upload_CdnFile, error) {
	if c.MD != nil && c.MD.IsBot {
		c.Logger.Errorf("upload.getCdnFile - error: bot")
		return nil, mtproto.ErrBotMethodInvalid
	}
	offset := cdnOffset(in.GetOffset_INT64(), in.GetOffset_INT32())
	if len(helpdesk.PublicKeys()) == 0 {
		if err := cdn.CheckRange(offset, in.GetLimit()); err != nil {
			c.Logger.Errorf("upload.getCdnFile - error: %v", err)
			return nil, err
		}
		c.Logger.Errorf("upload.getCdnFile - error: CDN is not published")
		return nil, mtproto.ErrFileTokenInvalid
	}
	out, err := cdn.Shared().Get(in.GetFileToken(), offset, in.GetLimit())
	if err != nil {
		c.Logger.Errorf("upload.getCdnFile - error: %v", err)
		return nil, err
	}
	return out, nil
}
