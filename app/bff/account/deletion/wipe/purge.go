package wipe

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/mediapurge"
)

func releaseWipedBoxes(ctx context.Context, owner int64, boxes []Box) {
	out := make([]mediapurge.Box, 0, len(boxes))
	for _, b := range boxes {
		box := mediapurge.Box{Kind: mediapurge.BoxUser, Owner: owner, ID: b.ID}
		if b.PhotoID != 0 {
			box.Media = append(box.Media, mediapurge.Media{Kind: mediapurge.KindPhoto, ID: b.PhotoID})
		}
		if b.DocumentID != 0 {
			box.Media = append(box.Media, mediapurge.Media{Kind: mediapurge.KindDocument, ID: b.DocumentID})
		}
		out = append(out, box)
	}
	mediapurge.ReleaseKnown(ctx, out)
}
