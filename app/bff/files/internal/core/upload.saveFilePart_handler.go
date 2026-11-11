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
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/pkg/uploadledger"
)

// UploadSaveFilePart
// upload.saveFilePart#b304a621 file_id:long file_part:int bytes:bytes = Bool;
func (c *FilesCore) UploadSaveFilePart(in *mtproto.TLUploadSaveFilePart) (*mtproto.Bool, error) {
	subject := uploadSubject(c)
	hold, err := chargeUploadPart(subject, in.Bytes)
	if err != nil {
		c.Logger.Errorf("upload.saveFilePart - budget")
		return nil, mtproto.NewErrFloodWaitX(uploadledger.WaitOf(err))
	}
	_, err = c.svcCtx.Dao.DfsClient.DfsWriteFilePartData(c.ctx, &dfs.TLDfsWriteFilePartData{
		Creator:        c.MD.PermAuthKeyId,
		FileId:         in.FileId,
		FilePart:       in.FilePart,
		Bytes:          in.Bytes,
		Big:            false,
		FileTotalParts: nil,
	})
	if led := uploadledger.Shared(); led != nil {
		led.Cancel(hold)
	}
	if err != nil {
		c.Logger.Errorf("upload.saveFilePart - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}

func uploadSubject(c *FilesCore) int64 {
	if c == nil || c.MD == nil {
		return 0
	}
	if c.MD.UserId != 0 {
		return c.MD.UserId
	}
	return c.MD.PermAuthKeyId
}

func chargeUploadPart(subject int64, body []byte) (int64, error) {
	led := uploadledger.Shared()
	if led == nil || subject == 0 || len(body) == 0 {
		return 0, nil
	}
	if err := led.ChargeNetwork(subject, int64(len(body))); err != nil {
		return 0, err
	}
	return led.Reserve(subject, int64(len(body)))
}
