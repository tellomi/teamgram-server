package mysql_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/mediapurge"
)

func noteUserBox(ctx context.Context, do *dataobject.MessagesDO) {
	if do == nil || do.Deleted {
		return
	}
	mediapurge.NoteUser(ctx, do.UserId, do.UserMessageBoxId, do.MessageData)
}

func releaseUserBoxes(ctx context.Context, userID int64, rows []dataobject.MessagesDO) {
	if len(rows) == 0 {
		return
	}
	boxes := make([]mediapurge.UserBox, 0, len(rows))
	for _, row := range rows {
		boxes = append(boxes, mediapurge.UserBox{ID: row.UserMessageBoxId, MessageData: row.MessageData})
	}
	mediapurge.ReleaseUser(ctx, userID, boxes)
}
