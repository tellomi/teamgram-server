/*
 * WARNING! All changes made in this file will be lost!
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright 2022 Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package files_helper

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/files/plugin"
	"github.com/teamgram/teamgram-server/app/bff/phone/secretchat"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/zeromicro/go-zero/zrpc"
)

type (
	Config = config.Config
)

func init() {
	zrpc.DontLogContentForMethod("/mtproto.RPCFiles/upload_saveFilePart")
	zrpc.DontLogContentForMethod("/mtproto.RPCFiles/upload_saveBigFilePart")

	zrpc.DontLogClientContentForMethod("/dfs.RPCDfs/dfs_writeFilePartData")
	zrpc.DontLogClientContentForMethod("/dfs.RPCDfs/dfs_downloadFile")
}

func New(c Config, plugin1 plugin.ThumbFilesPlugin, plugin2 plugin.StreamFilesPlugin) *service.Service {
	svcCtx := svc.NewServiceContext(c, plugin1, plugin2)
	secretchat.Materialize = func(ctx context.Context, creator int64, chatID int32, file *mtproto.InputEncryptedFile) (*mtproto.EncryptedFile, error) {
		if svcCtx == nil || svcCtx.DfsClient == nil || file == nil {
			return nil, mtproto.ErrFileIdInvalid
		}
		out, err := svcCtx.DfsClient.DfsUploadEncryptedFileV2(ctx, &dfs.TLDfsUploadEncryptedFileV2{
			Creator: creator,
			File:    file,
		})
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, mtproto.ErrFileIdInvalid
		}
		out.KeyFingerprint = file.GetKeyFingerprint()
		if out.GetDcId() == 0 {
			out.DcId = 1
		}
		_ = chatID
		return out, nil
	}
	return service.New(svcCtx)
}
