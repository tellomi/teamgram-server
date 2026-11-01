package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/readtime"
)

// readHit is one reader and the Unix second to show. Date 0 means unknown.
type readHit struct {
	UserID int64
	Date   int64
}

// datesFromHits builds the client vector. A zero date is omitted: UTC+8
// clients render the epoch as 08:00.
func datesFromHits(hits []readHit, excludeUserID int64) []*mtproto.ReadParticipantDate {
	out := make([]*mtproto.ReadParticipantDate, 0, len(hits))
	for _, h := range hits {
		if h.UserID == 0 || h.UserID == excludeUserID {
			continue
		}
		date := readtime.Pick(h.Date)
		if date == 0 {
			continue
		}
		out = append(out, mtproto.MakeTLReadParticipantDate(&mtproto.ReadParticipantDate{
			UserId: h.UserID,
			Date:   date,
		}).To_ReadParticipantDate())
	}
	return out
}
