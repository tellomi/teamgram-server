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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
)

// maxFilePartBytes is the 512 KiB cap documented on model.CheckFilePartSize.
// The helper's FILE_PART_TOO_BIG branch is unreachable for a positive length
// (anything larger than 512 KiB fails the "512 KiB divides part size" check
// first), so a short remainder — which must not run that helper — still
// rejects an oversized body with the error the helper names for this cap.
const maxFilePartBytes = 524288

// DfsWriteFilePartData
// dfs.writeFilePartData flags:# creator:long file_id:long bytes:bytes big:flags.0?true file_total_parts:flags.1?int = Bool;
func (c *DfsCore) DfsWriteFilePartData(in *dfs.TLDfsWriteFilePartData) (*mtproto.Bool, error) {
	var (
		err error
	)

	if err = validateFilePartWrite(in); err != nil {
		c.Logger.Errorf("dfs.writeFilePartData - error: %v", err)
		return nil, err
	}

	err = c.svcCtx.Dao.WriteFilePartData(c.ctx, in.Creator, in.FileId, in.FilePart, in.Bytes)
	if err != nil {
		c.Logger.Errorf("dfs.writeFilePartData - error: %v", err)
		return nil, err
	}

	if in.FilePart == 0 {
		err = c.svcCtx.Dao.SetFileInfo(c.ctx, &model.DfsFileInfo{
			Creator:           in.Creator,
			FileId:            in.FileId,
			Big:               in.Big,
			FileName:          "",
			FileTotalParts:    int(in.GetFileTotalParts().GetValue()),
			FirstFilePartSize: len(in.Bytes),
			FilePartSize:      0,
			LastFilePartSize:  0,
			MimeType:          "",
			Mtime:             time.Now().Unix(),
		})

		// TODO(@benqi): error
		if err != nil {
			c.Logger.Errorf("dfs.writeFilePartData - error: %v", err)
			return nil, mtproto.ErrFilePartInvalid
		}
	} else if in.FilePart == 1 {
		err = c.svcCtx.Dao.SetFileInfo(c.ctx, &model.DfsFileInfo{
			Creator:      in.Creator,
			FileId:       in.FileId,
			FilePartSize: len(in.Bytes),
		})

		// TODO(@benqi): error
		if err != nil {
			c.Logger.Errorf("dfs.writeFilePartData - error: %v", err)
			return nil, mtproto.ErrFilePartInvalid
		}
	}

	if in.GetFileTotalParts() != nil {
		if in.GetFileTotalParts().GetValue() == in.FilePart+1 {
			err = c.svcCtx.Dao.SetFileInfo(
				c.ctx,
				&model.DfsFileInfo{
					Creator:          in.Creator,
					FileId:           in.FileId,
					LastFilePartSize: len(in.Bytes),
				})

			// TODO(@benqi): error
			if err != nil {
				c.Logger.Errorf("dfs.writeFilePartData - error: %v", err)
				return nil, mtproto.ErrFilePartInvalid
			}
		}
	}

	return mtproto.BoolTrue, nil
}

// validateFilePartWrite applies the official part rules before anything is
// stored. saveBigFilePart sends file_total_parts, so every part except the
// last one must pass model.CheckFilePartSize. saveFilePart does not send a
// total: the part may be the only part (a 50 KiB JPEG) or the remainder,
// and parts may arrive out of order, so the size helper is not applied.
// A remainder may be shorter than the standard part. It may not be larger
// than 512 KiB.
func validateFilePartWrite(in *dfs.TLDfsWriteFilePartData) error {
	if len(in.GetBytes()) == 0 {
		return mtproto.ErrFilePartEmpty
	}
	if err := model.CheckFilePart(in.GetFilePart()); err != nil {
		return err
	}
	n := int32(len(in.GetBytes()))
	if filePartIsRemainder(in) {
		if n > maxFilePartBytes {
			return mtproto.ErrFilePartTooBig
		}
		return nil
	}
	return model.CheckFilePartSize(n)
}

func filePartIsRemainder(in *dfs.TLDfsWriteFilePartData) bool {
	total := in.GetFileTotalParts()
	if total == nil || total.GetValue() <= 0 {
		return true
	}
	return in.GetFilePart()+1 == total.GetValue()
}
