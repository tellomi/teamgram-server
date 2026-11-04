/*
 * WARNING! All changes made in this file will be lost!
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2022-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teagramio (teagram.io@gmail.com)
 */

package http

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ParseDfsFileName reads creator_fileId.ext. A bare name such as x.mp4 is
// rejected: it is a document filename, not an object key.
func ParseDfsFileName(name string) (creator, fileID int64, err error) {
	name = path.Base(strings.TrimSpace(name))
	stem := name
	if i := strings.LastIndex(name, "."); i > 0 {
		stem = name[:i]
	}
	parts := strings.Split(stem, "_")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid fileName %s", name)
	}
	if creator, err = strconv.ParseInt(parts[0], 10, 64); err != nil {
		return 0, 0, fmt.Errorf("invalid fileName %s", name)
	}
	if fileID, err = strconv.ParseInt(parts[1], 10, 64); err != nil {
		return 0, 0, fmt.Errorf("invalid fileName %s", name)
	}
	if fileID == 0 {
		return 0, 0, fmt.Errorf("invalid fileName %s", name)
	}
	return creator, fileID, nil
}

func GetDfsFile(ctx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			err     error
			creator int64
			fileId  int64
		)

		var (
			fileName struct {
				FileName string `path:"file"`
			}
		)

		err = httpx.ParsePath(r, &fileName)
		if err != nil {
			logx.Errorf("getDfsFile - error: %v", err)
			httpx.Error(w, err)
			return
		}

		creator, fileId, err = ParseDfsFileName(fileName.FileName)
		if err != nil {
			logx.Errorf("getDfsFile - error: %v", err)
			http.NotFound(w, r)
			return
		}

		f, err := DfsOpenFile(ctx, creator, fileId)
		if err != nil {
			logx.Errorf("getDfsFile(%d, %d) - error: %v", creator, fileId, err)
			http.NotFound(w, r)
			return
		}

		http.ServeContent(w, r, fileName.FileName, f.Modtime, f.ReadSeeker)
	}
}

func DfsOpenFile(ctx *svc.ServiceContext, creatorId, fileId int64) (f *model.DfsHttpFileInfo, err error) {
	fileInfo, err := ctx.Dao.GetFileInfo(context.Background(), creatorId, fileId)
	if err != nil {
		return nil, err
	}

	return &model.DfsHttpFileInfo{
		ReadSeeker: ctx.Dao.NewSSDBReader(fileInfo),
		Name:       fileInfo.FileName,
		Modtime:    time.Unix(fileInfo.Mtime, 0),
	}, nil
}
