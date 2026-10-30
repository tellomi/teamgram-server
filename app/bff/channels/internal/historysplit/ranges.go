// Package historysplit answers messages.getSplitRanges.
//
// https://core.telegram.org/method/messages.getSplitRanges
package historysplit

import "github.com/teamgram/proto/mtproto"

// Ranges is the message-id ranges a client scans when history is split across
// databases. This server keeps history in one database, so the vector is empty.
func Ranges() *mtproto.Vector_MessageRange {
	return &mtproto.Vector_MessageRange{Datas: []*mtproto.MessageRange{}}
}
