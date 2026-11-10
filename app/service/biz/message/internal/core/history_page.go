package core

import "github.com/teamgram/proto/mtproto"

// trimNewestFirst applies a positive add_offset to a newest-first page.
// The fetch asked for addOffset+limit rows. The first addOffset of those
// are the ones sitting closest to offset_id; official clients skip them
// and still receive limit messages. addOffset 0 only caps the page.
func trimNewestFirst(boxes []*mtproto.MessageBox, addOffset, limit int32) []*mtproto.MessageBox {
	if addOffset > 0 {
		if int(addOffset) >= len(boxes) {
			return nil
		}
		boxes = boxes[addOffset:]
	}
	if limit >= 0 && len(boxes) > int(limit) {
		boxes = boxes[:limit]
	}
	return boxes
}
