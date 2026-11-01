// Package searchbox turns stored messages into the shapes the shared-media
// and mention RPCs return.
//
// https://core.telegram.org/method/messages.getSearchResultsCalendar
// https://core.telegram.org/method/messages.getSearchResultsPositions
// https://core.telegram.org/method/messages.getRecentLocations
// https://core.telegram.org/method/messages.searchSentMedia
// https://core.telegram.org/method/messages.readMentions
package searchbox

import (
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
)

const (
	maxCalendarDays = 100
	maxPositions    = 100
	maxSentMedia    = 100
	daySeconds      = int32(86400)
	liveForever     = int32(0x7fffffff)
)

// Row is one stored message, newest-first when callers build a list.
type Row struct {
	ID      int32
	Date    int32
	Sender  int64
	Text    string
	Live    bool
	Period  int32
	TopID   int32
	Message *mtproto.Message
}

// Calendar is messages.searchResultsCalendar without the user/chat lists.
type Calendar struct {
	Inexact  bool
	Count    int32
	MinDate  int32
	MinMsgID int32
	Periods  []Period
	Messages []*mtproto.Message
}

// Period is one UTC day.
type Period struct {
	Date     int32
	MinMsgID int32
	MaxMsgID int32
	Count    int32
}

// Position is one scrollbar sample.
type Position struct {
	MsgID  int32
	Date   int32
	Offset int32
}

// MediaKind says which message-service query can satisfy a filter.
type MediaKind struct {
	Type   int32
	Pinned bool
}

// MediaFor reports the stored message_filter_type for a filter.
// inputMessagesFilterEmpty and inputMessagesFilterMyMentions are rejected.
// Filters this server does not index (geo, contacts, voice-only, round video)
// are rejected the same way.
func MediaFor(filter *mtproto.MessagesFilter) (MediaKind, error) {
	switch mtproto.FromMessagesFilter(filter) {
	case mtproto.FilterPhotos:
		return MediaKind{Type: mtproto.MEDIA_PHOTOS_ONLY}, nil
	case mtproto.FilterVideo:
		return MediaKind{Type: mtproto.MEDIA_VIDEOS_ONLY}, nil
	case mtproto.FilterPhotoVideo:
		return MediaKind{Type: mtproto.MEDIA_PHOTOVIDEO}, nil
	case mtproto.FilterDocument:
		return MediaKind{Type: mtproto.MEDIA_FILE}, nil
	case mtproto.FilterUrl:
		return MediaKind{Type: mtproto.MEDIA_URL}, nil
	case mtproto.FilterGif:
		return MediaKind{Type: mtproto.MEDIA_GIF}, nil
	case mtproto.FilterMusic:
		return MediaKind{Type: mtproto.MEDIA_MUSIC}, nil
	case mtproto.FilterChatPhotos:
		return MediaKind{Type: mtproto.MEDIA_CHAT_PHOTO}, nil
	case mtproto.FilterPhoneCalls:
		return MediaKind{Type: mtproto.MEDIA_PHONE_CALL}, nil
	case mtproto.FilterRoundVoice:
		return MediaKind{Type: mtproto.MEDIA_AUDIO}, nil
	case mtproto.FilterPinned:
		return MediaKind{Pinned: true}, nil
	default:
		return MediaKind{}, mtproto.ErrFilterNotSupported
	}
}

// Calendar groups rows into UTC days, newest day first.
// offsetID keeps ids strictly older than that id. offsetDate keeps dates
// at or before that unix time. A day past maxCalendarDays, or a scan that
// stopped early, sets Inexact. The last returned day can be partial.
func BuildCalendar(rows []Row, offsetID, offsetDate int32, truncated bool) Calendar {
	rows = olderThan(rows, offsetID, offsetDate)
	var out Calendar
	if len(rows) == 0 {
		return out
	}
	var cur *Period
	var curPreview *mtproto.Message
	flush := func() {
		if cur == nil {
			return
		}
		out.Periods = append(out.Periods, *cur)
		if curPreview != nil {
			out.Messages = append(out.Messages, curPreview)
		}
		out.Count += cur.Count
		cur = nil
		curPreview = nil
	}
	for _, row := range rows {
		day := dayStart(row.Date)
		if cur != nil && cur.Date != day {
			flush()
			if len(out.Periods) >= maxCalendarDays {
				out.Inexact = true
				break
			}
		}
		if cur == nil {
			cur = &Period{Date: day, MinMsgID: row.ID, MaxMsgID: row.ID, Count: 1}
			curPreview = row.Message
			continue
		}
		cur.Count++
		if row.ID < cur.MinMsgID {
			cur.MinMsgID = row.ID
		}
		if row.ID > cur.MaxMsgID {
			cur.MaxMsgID = row.ID
		}
	}
	if !out.Inexact {
		flush()
	} else if cur != nil && len(out.Periods) < maxCalendarDays {
		// The day we were filling when the cap hit is partial.
		flush()
	}
	if truncated {
		out.Inexact = true
	}
	if n := len(out.Periods); n > 0 {
		last := out.Periods[n-1]
		out.MinDate = last.Date
		out.MinMsgID = last.MinMsgID
	}
	return out
}

// BuildPositions samples rows for the shared-media scrollbar.
// offset is the index from the newest matching message.
func BuildPositions(rows []Row, offsetID, limit int32) (count int32, positions []Position) {
	rows = olderThan(rows, offsetID, 0)
	count = int32(len(rows))
	if count == 0 {
		return 0, nil
	}
	if limit <= 0 || limit > maxPositions {
		limit = maxPositions
	}
	if limit > count {
		limit = count
	}
	positions = make([]Position, 0, limit)
	if limit == 1 {
		positions = append(positions, Position{MsgID: rows[0].ID, Date: rows[0].Date, Offset: 0})
		return count, positions
	}
	var prev int32 = -1
	for i := int32(0); i < limit; i++ {
		idx := i * (count - 1) / (limit - 1)
		if idx == prev {
			continue
		}
		prev = idx
		row := rows[idx]
		positions = append(positions, Position{MsgID: row.ID, Date: row.Date, Offset: idx})
	}
	return count, positions
}

// PickLive keeps the newest unexpired live location per sender.
func PickLive(rows []Row, now, limit int32) []Row {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	seen := map[int64]bool{}
	var out []Row
	for _, row := range rows {
		if !row.Live || row.Sender == 0 || seen[row.Sender] {
			continue
		}
		if !liveOpen(row, now) {
			continue
		}
		seen[row.Sender] = true
		out = append(out, row)
		if int32(len(out)) >= limit {
			break
		}
	}
	return out
}

// SentMedia keeps rows whose text contains q and returns at most limit.
func SentMedia(rows []Row, q string, limit int32) []Row {
	if limit <= 0 || limit > maxSentMedia {
		limit = maxSentMedia
	}
	q = strings.ToLower(strings.TrimSpace(q))
	var out []Row
	for _, row := range rows {
		if q != "" && !strings.Contains(strings.ToLower(row.Text), q) {
			continue
		}
		out = append(out, row)
		if int32(len(out)) >= limit {
			break
		}
	}
	return out
}

// MentionIDs returns mention message ids. topID limits the list to one forum topic.
func MentionIDs(rows []Row, topID int32) []int32 {
	var ids []int32
	for _, row := range rows {
		if topID != 0 && row.TopID != topID && row.ID != topID {
			continue
		}
		if row.ID == 0 {
			continue
		}
		ids = append(ids, row.ID)
	}
	return ids
}

func olderThan(rows []Row, offsetID, offsetDate int32) []Row {
	if offsetID <= 0 && offsetDate <= 0 {
		return rows
	}
	out := make([]Row, 0, len(rows))
	for _, row := range rows {
		if offsetID > 0 && row.ID >= offsetID {
			continue
		}
		if offsetDate > 0 && row.Date > offsetDate {
			continue
		}
		out = append(out, row)
	}
	return out
}

func dayStart(unix int32) int32 {
	if unix < 0 {
		unix = 0
	}
	// Anchor the day on UTC so two servers agree.
	t := time.Unix(int64(unix), 0).UTC()
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return int32(start.Unix())
}

func liveOpen(row Row, now int32) bool {
	if row.Period >= liveForever {
		return true
	}
	if row.Period <= 0 {
		return false
	}
	end := int64(row.Date) + int64(row.Period)
	return end > int64(now)
}

// HashIDs is the cache hash clients send back for getRecentLocations.
func HashIDs(ids []int32) int64 {
	var hash int64
	for _, id := range ids {
		hash = mix(hash, int64(id))
	}
	return hash
}

func mix(hash, id int64) int64 {
	hash ^= id >> 21
	hash ^= id << 35
	hash ^= id >> 4
	return hash + id
}
