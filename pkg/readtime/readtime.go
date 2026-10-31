// Package readtime turns a peer's read mark into a Unix timestamp clients can
// show. A zero timestamp is the Unix epoch, which China-timezone clients render
// as 08:00 on every message.
package readtime

import (
	"math"
	"time"
)

// UpsertTail keeps the first positive read time and fills a zero one.
// A later read of the same checkpoint must not wipe the original time, and a
// row that was stored as 0 must pick up a real time the next time it is written.
const UpsertTail = `on duplicate key update read_outbox_max_date = if(read_outbox_max_date > 0, read_outbox_max_date, values(read_outbox_max_date))`

// PositionalUpsert inserts one read checkpoint.
func PositionalUpsert() string {
	return `insert into message_read_outbox(user_id, peer_dialog_id, read_user_id, read_outbox_max_id, read_outbox_max_date) values (?, ?, ?, ?, ?) ` + UpsertTail
}

// NamedUpsert is PositionalUpsert with sqlx named arguments.
func NamedUpsert() string {
	return `insert into message_read_outbox(user_id, peer_dialog_id, read_user_id, read_outbox_max_id, read_outbox_max_date) values (:user_id, :peer_dialog_id, :read_user_id, :read_outbox_max_id, :read_outbox_max_date) ` + UpsertTail
}

// SelectHitsSQL returns one row per reader: the earliest positive read time
// among checkpoints that already cover msg_id. When that column is 0, the
// row's updated_at is the time the checkpoint was written.
func SelectHitsSQL(oneReader, withUpdated bool) string {
	dateExpr := `ifnull(min(case when read_outbox_max_date > 0 then read_outbox_max_date end), 0)`
	if withUpdated {
		dateExpr = `ifnull(coalesce(min(case when read_outbox_max_date > 0 then read_outbox_max_date end), max(cast(unix_timestamp(updated_at) as signed))), 0)`
	}
	q := `select read_user_id, ` + dateExpr + ` as read_date from message_read_outbox where user_id = ? and peer_dialog_id = ? and read_outbox_max_id >= ? and read_user_id <> user_id`
	if oneReader {
		q += ` and read_user_id = ?`
	}
	q += ` group by read_user_id`
	return q
}

// Stamp is the Unix second to persist. It is never 0.
func Stamp(now int64) int64 {
	if sec := asSeconds(now); sec > 0 {
		return sec
	}
	n := time.Now().Unix()
	if n <= 0 {
		return 1
	}
	return n
}

// Combine prefers the stored read time and falls back to when the row was written.
func Combine(stored, updated int64) int64 {
	if sec := asSeconds(stored); sec > 0 {
		return sec
	}
	if sec := asSeconds(updated); sec > 0 {
		return sec
	}
	return 0
}

// Pick converts a Unix second to the int32 clients decode. Zero and values
// that do not fit in a positive int32 become 0 so the caller can omit them
// instead of sending the epoch.
func Pick(v int64) int32 {
	sec := asSeconds(v)
	if sec <= 0 || sec > math.MaxInt32 {
		return 0
	}
	return int32(sec)
}

func asSeconds(v int64) int64 {
	if v <= 0 {
		return 0
	}
	// Milliseconds stored by mistake (a current Unix second fits in 10 digits).
	if v > 10_000_000_000 {
		v /= 1000
	}
	if v <= 0 {
		return 0
	}
	return v
}
