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
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strings"

	"github.com/teamgram/marmota/pkg/threading2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
)

// DfsUploadEncryptedFileV2
// dfs.uploadEncryptedFileV2 creator:long file:InputEncryptedFile = EncryptedFile;
//
// creator is the permanent auth key used by upload.saveFilePart. The assembled
// object is encryptedfiles/<id>.dat, which upload.getFile reads for
// inputEncryptedFileLocation. The Redis part cache covers the download until
// that write finishes.
func (c *DfsCore) DfsUploadEncryptedFileV2(in *dfs.TLDfsUploadEncryptedFileV2) (*mtproto.EncryptedFile, error) {
	file := in.GetFile()
	// Reject a bad descriptor before touching the part cache.
	if err := validateEncryptedSource(file, &model.DfsFileInfo{
		FileTotalParts:    int(file.GetParts()),
		FirstFilePartSize: 1,
		FilePartSize:      1,
	}); err != nil {
		c.Logger.Errorf("dfs.uploadEncryptedFile - error: %v", err)
		return nil, err
	}
	fileInfo, err := c.svcCtx.Dao.GetFileInfo(c.ctx, in.GetCreator(), file.GetId())
	if err != nil {
		c.Logger.Errorf("dfs.uploadEncryptedFile - error: %v", err)
		if errors.Is(err, model.ErrorDfsFileNotFound) {
			return nil, mtproto.NewFilePartXMissing(0)
		}
		return nil, err
	}
	if err = validateEncryptedSource(file, fileInfo); err != nil {
		c.Logger.Errorf("dfs.uploadEncryptedFile - error: %v", err)
		return nil, err
	}
	if file.GetPredicateName() == mtproto.Predicate_inputEncryptedFileUploaded {
		digest, err := c.encryptedDigest(fileInfo)
		if err != nil {
			return nil, err
		}
		if err = encryptedChecksum(file, digest); err != nil {
			c.Logger.Errorf("dfs.uploadEncryptedFile - md5: %v", err)
			return nil, err
		}
	}

	encryptedFileId := c.svcCtx.Dao.IDGenClient2.NextId(c.ctx)
	accessHash := int64(mtproto.CRC32_storage_filePartial)<<32 | int64(rand.Uint32())
	c.svcCtx.Dao.SetCacheFileInfo(c.ctx, encryptedFileId, fileInfo)
	path := fmt.Sprintf("%d.dat", encryptedFileId)
	// The RPC context is canceled when this method returns. The object write
	// has to outlive that, or upload.getFile loses the bytes after the part
	// cache expires.
	threading2.GoSafeContext(context.WithoutCancel(c.ctx), func(ctx context.Context) {
		_, err2 := c.svcCtx.Dao.PutEncryptedFile(ctx, path, c.svcCtx.Dao.NewSSDBReader(fileInfo))
		if err2 != nil {
			c.Logger.Errorf("dfs.uploadEncryptedFile - error: %v", err2)
		}
	})

	size := fileInfo.GetFileSize()
	return mtproto.MakeTLEncryptedFile(&mtproto.EncryptedFile{
		Id:             encryptedFileId,
		AccessHash:     accessHash,
		Size2_INT32:    int32(size),
		Size2_INT64:    size,
		DcId:           1,
		KeyFingerprint: file.GetKeyFingerprint(),
	}).To_EncryptedFile(), nil
}

func (c *DfsCore) encryptedDigest(info *model.DfsFileInfo) (string, error) {
	sum := md5.New()
	n, err := io.Copy(sum, c.svcCtx.Dao.NewSSDBReader(info))
	if err != nil {
		c.Logger.Errorf("dfs.uploadEncryptedFile - read: %v", err)
		return "", mtproto.ErrFilePartInvalid
	}
	if info.GetFileSize() > 0 && n != info.GetFileSize() {
		return "", mtproto.NewFilePartXMissing(0)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// validateEncryptedSource checks the uploaded part descriptor against the
// parts upload.saveFilePart already stored. digest is unused here; the MD5
// of the bytes is checked separately so a bad part count does not read them.
func validateEncryptedSource(file *mtproto.InputEncryptedFile, info *model.DfsFileInfo) error {
	if file == nil || file.GetId() == 0 {
		return mtproto.ErrFileEmtpy
	}
	switch file.GetPredicateName() {
	case mtproto.Predicate_inputEncryptedFileUploaded, mtproto.Predicate_inputEncryptedFileBigUploaded:
	default:
		return mtproto.ErrFileIdInvalid
	}
	if err := model.CheckFileParts(file.GetParts()); err != nil {
		return err
	}
	if info == nil {
		return mtproto.NewFilePartXMissing(0)
	}
	if info.FileTotalParts != int(file.GetParts()) {
		return mtproto.ErrFilePartsInvalid
	}
	if info.FirstFilePartSize <= 0 {
		return mtproto.NewFilePartXMissing(0)
	}
	if info.FileTotalParts > 1 && info.FilePartSize <= 0 {
		return mtproto.ErrFilePartSizeInvalid
	}
	return nil
}

func encryptedChecksum(file *mtproto.InputEncryptedFile, digest string) error {
	if file.GetPredicateName() != mtproto.Predicate_inputEncryptedFileUploaded {
		return nil
	}
	if len(file.GetMd5Checksum()) != 32 || !strings.EqualFold(digest, file.GetMd5Checksum()) {
		return mtproto.ErrCheckSumInvalid
	}
	return nil
}
