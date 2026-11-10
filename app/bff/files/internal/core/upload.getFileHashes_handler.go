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
	"crypto/sha256"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/files/cdn"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

// UploadGetFileHashes
// upload.getFileHashes#9156982a location:InputFileLocation offset:long = Vector<FileHash>;
// upload.getFileHashes#c7025931 location:InputFileLocation offset:int = Vector<FileHash>;
// https://core.telegram.org/method/upload.getFileHashes
//
// Returns SHA-256 of the plaintext parts upload.getFile would serve, using
// the same 128 KiB part size and offset filter as upload.getCdnFileHashes.
// CDN hashes are of ciphertext; this method hashes the file itself.
func (c *FilesCore) UploadGetFileHashes(in *mtproto.TLUploadGetFileHashes) (*mtproto.Vector_FileHash, error) {
	offset := cdnOffset(in.GetOffset_INT64(), in.GetOffset_INT32())
	if offset < 0 {
		c.Logger.Errorf("upload.getFileHashes - offset %d", offset)
		return nil, mtproto.ErrOffsetInvalid
	}
	location := in.GetLocation()
	if location == nil || location.GetPredicateName() == "" {
		c.Logger.Errorf("upload.getFileHashes - invalid location")
		return nil, mtproto.ErrLocationInvalid
	}
	switch location.GetPredicateName() {
	case mtproto.Predicate_inputFileLocation:
		err := mtproto.ErrInputRequestInvalid
		c.Logger.Errorf("upload.getFileHashes - error: %v inputFileLocation", err)
		return nil, err
	case mtproto.Predicate_inputStickerSetThumb:
		loc, err := c.stickerSetThumb(location)
		if err != nil {
			c.Logger.Errorf("upload.getFileHashes - error: %v inputStickerSetThumb", err)
			return nil, err
		}
		location = loc
	case mtproto.Predicate_inputGroupCallStream:
		// A live stream has no stable file to hash.
		return &mtproto.Vector_FileHash{}, nil
	case mtproto.Predicate_inputEncryptedFileLocation,
		mtproto.Predicate_inputDocumentFileLocation,
		mtproto.Predicate_inputSecureFileLocation,
		mtproto.Predicate_inputTakeoutFileLocation,
		mtproto.Predicate_inputPhotoFileLocation,
		mtproto.Predicate_inputPhotoLegacyFileLocation,
		mtproto.Predicate_inputPeerPhotoFileLocation:
	default:
		c.Logger.Errorf("upload.getFileHashes - invalid location %s", location.GetPredicateName())
		return nil, mtproto.ErrLocationInvalid
	}
	if err := c.authorize(location); err != nil {
		c.Logger.Errorf("upload.getFileHashes - access: %v", err)
		return nil, err
	}
	if c.mediaExpired(location) {
		c.Logger.Infof("upload.getFileHashes - %s %d expired by media retention", location.GetPredicateName(), location.GetId())
		return nil, mtproto.ErrFileIdInvalid
	}
	hashes, err := c.locationHashes(location, offset)
	if err != nil {
		c.Logger.Errorf("upload.getFileHashes - error: %v", err)
		return nil, err
	}
	return &mtproto.Vector_FileHash{Datas: hashes}, nil
}

// locationHashes reads the object upload.getFile would return and splits it
// into the same SHA-256 parts as cdn.PartHashes.
func (c *FilesCore) locationHashes(location *mtproto.InputFileLocation, offset int64) ([]*mtproto.FileHash, error) {
	if location.GetPredicateName() == mtproto.Predicate_inputPhotoFileLocation {
		if body, _, ok := chatbox.WebPhoto(location.GetId()); ok {
			return cdn.PartHashes(body, offset)
		}
	}
	if body, ok := fullSeeded(location); ok {
		return cdn.PartHashes(body, offset)
	}
	if file, ok := seededDocumentFile(location, 0, 0); ok {
		return cdn.PartHashes(file.GetBytes(), offset)
	}
	return hashFileParts(offset, func(off int64, n int32) ([]byte, error) {
		part, err := c.download(location, off, n)
		if err != nil || part == nil {
			return nil, err
		}
		return part.GetBytes(), nil
	})
}

// hashFileParts walks PartBytes slices from the first part boundary at or
// after offset. A short read ends the file. The result matches cdn.PartHashes
// on the same bytes.
func hashFileParts(offset int64, read func(off int64, n int32) ([]byte, error)) ([]*mtproto.FileHash, error) {
	if offset < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	start := int64(0)
	if offset > 0 {
		part := int64(cdn.PartBytes)
		start = ((offset + part - 1) / part) * part
	}
	var out []*mtproto.FileHash
	for off := start; ; {
		chunk, err := read(off, cdn.PartBytes)
		if err != nil {
			return nil, err
		}
		if len(chunk) > cdn.PartBytes {
			chunk = chunk[:cdn.PartBytes]
		}
		if len(chunk) == 0 {
			return out, nil
		}
		sum := sha256.Sum256(chunk)
		out = append(out, mtproto.MakeTLFileHash(&mtproto.FileHash{
			Offset_INT64: off,
			Limit:        int32(len(chunk)),
			Hash:         append([]byte(nil), sum[:]...),
		}).To_FileHash())
		if len(chunk) < cdn.PartBytes {
			return out, nil
		}
		off += int64(len(chunk))
	}
}
