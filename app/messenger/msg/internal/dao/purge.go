package dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/mediapurge"
)

func releaseDeletedBoxes(ctx context.Context, userID int64, rows []dataobject.MessagesDO) {
	boxes := make([]mediapurge.UserBox, 0, len(rows))
	for _, row := range rows {
		boxes = append(boxes, mediapurge.UserBox{ID: row.UserMessageBoxId, MessageData: row.MessageData})
	}
	mediapurge.ReleaseUser(ctx, userID, boxes)
}
