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
	"unicode"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/secretchat"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

// uploadEncryptedPartsHook is set by tests. Production reads the parts saved
// by upload.saveFilePart / upload.saveBigFilePart and writes them to the
// encrypted-file location upload.getFile already downloads.
var uploadEncryptedPartsHook func(ctx context.Context, creator int64, file *mtproto.InputEncryptedFile) (*mtproto.EncryptedFile, error)

// MessagesUploadEncryptedFile
// messages.uploadEncryptedFile#5057c497 peer:InputEncryptedChat file:InputEncryptedFile = EncryptedFile;
// https://core.telegram.org/method/messages.uploadEncryptedFile
func (c *FilesCore) MessagesUploadEncryptedFile(in *mtproto.TLMessagesUploadEncryptedFile) (*mtproto.EncryptedFile, error) {
	if c.MD == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	// https://core.telegram.org/method/messages.uploadEncryptedFile — users only.
	if c.MD.IsBot {
		return nil, mtproto.ErrBotMethodInvalid
	}
	if _, err := secretchat.Default().Counterpart(c.MD.UserId, in.GetPeer()); err != nil {
		c.Logger.Errorf("messages.uploadEncryptedFile - peer: %v", err)
		return nil, err
	}

	file := in.GetFile()
	if file == nil || file.GetPredicateName() == "" || file.GetPredicateName() == mtproto.Predicate_inputEncryptedFileEmpty || file.GetId() == 0 {
		return nil, mtproto.ErrFileEmtpy
	}

	switch file.GetPredicateName() {
	case mtproto.Predicate_inputEncryptedFile:
		kept, chatID, ok := secretchat.Default().LookupEncrypted(file.GetId())
		if !ok || kept.GetAccessHash() != file.GetAccessHash() || chatID != in.GetPeer().GetChatId() {
			return nil, mtproto.ErrFileIdInvalid
		}
		return kept, nil
	case mtproto.Predicate_inputEncryptedFileUploaded:
		if err := checkEncryptedParts(file.GetParts()); err != nil {
			return nil, err
		}
		if !md5Hex(file.GetMd5Checksum()) {
			return nil, mtproto.ErrCheckSumInvalid
		}
	case mtproto.Predicate_inputEncryptedFileBigUploaded:
		if err := checkEncryptedParts(file.GetParts()); err != nil {
			return nil, err
		}
	default:
		return nil, mtproto.ErrFileIdInvalid
	}

	out, err := c.uploadEncryptedParts(file)
	if err != nil {
		c.Logger.Errorf("messages.uploadEncryptedFile - error: %v", err)
		return nil, err
	}
	if out == nil || out.GetId() == 0 {
		return nil, mtproto.ErrFileIdInvalid
	}
	// The client sends the 32-bit file-key fingerprint. The peer checks it
	// on the EncryptedFile attached to the secret message.
	out.KeyFingerprint = file.GetKeyFingerprint()
	if out.GetDcId() == 0 {
		out.DcId = 1
	}
	if out.GetPredicateName() == "" {
		out = mtproto.MakeTLEncryptedFile(out).To_EncryptedFile()
	}
	secretchat.Default().RememberEncrypted(in.GetPeer().GetChatId(), out)
	return out, nil
}

// uploadEncryptedParts stores parts under a new encrypted file id.
// creator is the permanent auth key, the same id upload.saveFilePart writes.
func (c *FilesCore) uploadEncryptedParts(file *mtproto.InputEncryptedFile) (*mtproto.EncryptedFile, error) {
	if uploadEncryptedPartsHook != nil {
		return uploadEncryptedPartsHook(c.ctx, c.MD.PermAuthKeyId, file)
	}
	return c.svcCtx.Dao.DfsClient.DfsUploadEncryptedFileV2(c.ctx, &dfs.TLDfsUploadEncryptedFileV2{
		Creator: c.MD.PermAuthKeyId,
		File:    file,
	})
}

// checkEncryptedParts is FILE_PARTS_INVALID when parts is outside 1..3000.
func checkEncryptedParts(parts int32) error {
	if parts < 1 || parts > 3000 {
		return mtproto.ErrFilePartsInvalid
	}
	return nil
}

func md5Hex(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			return false
		}
	}
	return true
}
