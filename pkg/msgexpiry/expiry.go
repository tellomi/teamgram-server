// Package msgexpiry hides messages whose ttl_period has elapsed.
// Both switches default off. Nothing here deletes an existing production row
// unless the process that calls Sweep is itself running, and SweepEnabled
// is the only gate for that.
package msgexpiry

import (
	"math"
	"os"

	"github.com/teamgram/proto/mtproto"
)

const (
	// EnvEnabled hides expired messages from history and get-message reads.
	// Unset or any value other than 1 leaves every stored row readable.
	EnvEnabled = "TELLOMI_MESSAGE_EXPIRY"
	// EnvSweep allows a caller to delete rows that are already expired.
	// The channel read path is the only caller, and only after this is 1.
	// There is no process-wide job and no migration that backfills a ttl.
	EnvSweep = "TELLOMI_MESSAGE_EXPIRY_SWEEP"
)

func Enabled() bool { return os.Getenv(EnvEnabled) == "1" }

func SweepEnabled() bool { return os.Getenv(EnvSweep) == "1" }

// Expired reports whether a message with this date and ttl is unreadable at now.
// ttl 0 never expires. A zero date does not expire, so a row that never stored
// a send time is left alone.
func Expired(date, ttl, now int32) bool {
	if ttl <= 0 || date <= 0 || now <= 0 {
		return false
	}
	if ttl > math.MaxInt32-date {
		return false
	}
	return now >= date+ttl
}

// FilterBoxes drops expired boxes when Enabled is on. Otherwise it returns
// the same slice. A nil box is dropped only while the switch is on.
func FilterBoxes(boxes []*mtproto.MessageBox, now int32) []*mtproto.MessageBox {
	if !Enabled() || len(boxes) == 0 {
		return boxes
	}
	out := make([]*mtproto.MessageBox, 0, len(boxes))
	for _, box := range boxes {
		if box == nil || Expired(boxDate(box), boxTTL(box), now) {
			continue
		}
		out = append(out, box)
	}
	return out
}

func boxTTL(box *mtproto.MessageBox) int32 {
	if box.GetTtlPeriod() > 0 {
		return box.GetTtlPeriod()
	}
	if box.GetMessage() != nil && box.GetMessage().GetTtlPeriod() != nil {
		return box.GetMessage().GetTtlPeriod().GetValue()
	}
	return 0
}

func boxDate(box *mtproto.MessageBox) int32 {
	if box.GetMessage() != nil {
		return box.GetMessage().GetDate()
	}
	return 0
}

// Item is one stored message a sweeper can see.
type Item struct {
	ID      int32
	Date    int32
	TTL     int32
	Deleted bool
}

// Plan returns ids that should be deleted. Already deleted ids are skipped,
// so calling Plan again on the same rows after those ids are marked deleted
// returns nothing.
func Plan(items []Item, now int32) []int32 {
	var ids []int32
	seen := map[int32]struct{}{}
	for _, item := range items {
		if item.Deleted || item.ID == 0 {
			continue
		}
		if !Expired(item.Date, item.TTL, now) {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		ids = append(ids, item.ID)
	}
	return ids
}
