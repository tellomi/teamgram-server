package channel

import (
	"context"
	"time"

	"github.com/teamgram/teamgram-server/app/service/mediapurge"
	"github.com/zeromicro/go-zero/core/logx"
)

func noteChannelMedia(ctx context.Context, channelID int64, msgID int32, body []byte) {
	s := mediapurge.Default()
	if s == nil || channelID == 0 || msgID == 0 {
		return
	}
	box := mediapurge.Box{
		Kind:  mediapurge.BoxChannel,
		Owner: channelID,
		ID:    msgID,
		Media: mediapurge.FromProto(body),
	}
	if err := s.Note(ctx, box); err != nil {
		logx.WithContext(ctx).Errorf("media purge: channel %d message %d: %v", channelID, msgID, err)
	}
}

func releaseChannelMedia(ctx context.Context, channelID int64, bodies map[int32][]byte) {
	s := mediapurge.Default()
	if s == nil || channelID == 0 || len(bodies) == 0 {
		return
	}
	boxes := make([]mediapurge.Box, 0, len(bodies))
	for id, body := range bodies {
		boxes = append(boxes, mediapurge.Box{
			Kind:  mediapurge.BoxChannel,
			Owner: channelID,
			ID:    id,
			Media: mediapurge.FromProto(body),
		})
	}
	if err := s.Release(ctx, boxes, time.Now()); err != nil {
		logx.WithContext(ctx).Errorf("media purge: channel %d: %v", channelID, err)
	}
}
