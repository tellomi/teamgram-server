// Package deletehistory is the messages.deleteHistory page planner.
//
// Official clients (Android MessagesController.deleteDialog, TelegramCore
// delete history) repeat the RPC while messages.affectedHistory.offset is
// positive, passing that offset back as max_id. The value falls as older
// messages are removed. min_date deletes messages newer than the timestamp
// and max_date deletes messages older than it. A non-user peer with either
// date is CHAT_REVOKE_DATE_UNSUPPORTED.
package deletehistory

import (
	"context"
	"math"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/metadata"
)

// Batch is how many messages one messages.deleteHistory call removes.
// Official servers cap a call and return a positive offset so the client
// calls again. One statement that names every id in a long chat fails in
// the middle; the RPC used to answer offset 0 anyway, and the messages
// that were not deleted came back the next time history was loaded.
const Batch = 100

// Query is one deleteHistory call after the TL flags are decoded.
type Query struct {
	MaxID     int32
	MinDate   int32
	MaxDate   int32
	MinSet    bool
	MaxSet    bool
	JustClear bool
}

// HasDates reports that the client set min_date or max_date.
func (q Query) HasDates() bool {
	return q.MinSet || q.MaxSet
}

// Row is one live message the page query returned, newest first.
type Row struct {
	ID     int32
	Date   int64
	DataID int64
}

// Select is the bounded history read for one page.
// Empty means the date window cannot match any message.
type Select struct {
	Empty       bool
	BeforeID    int32
	BeforeIDSet bool
	AfterDate   int64
	BeforeDate  int64
	Limit       int32
}

type rangeKey struct{}

// WithRange stores the date flags on ctx. The msg client copies them into
// gRPC metadata because TL_msg_deleteHistory has no date fields. A direct
// in-process call can read the value back with RangeFrom.
func WithRange(ctx context.Context, q Query) context.Context {
	return context.WithValue(ctx, rangeKey{}, q)
}

const (
	metaMin = "tl-delete-history-min"
	metaMax = "tl-delete-history-max"
)

// AppendOutgoing adds the date flags to the outgoing gRPC context.
// The context value set by WithRange is kept; this only adds metadata.
func AppendOutgoing(ctx context.Context) context.Context {
	q, ok := rangeFromValue(ctx)
	if !ok || !q.HasDates() {
		return ctx
	}
	var kv []string
	if q.MinSet {
		kv = append(kv, metaMin, strconv.FormatInt(int64(q.MinDate), 10))
	}
	if q.MaxSet {
		kv = append(kv, metaMax, strconv.FormatInt(int64(q.MaxDate), 10))
	}
	if len(kv) == 0 {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, kv...)
}

// RangeFrom reads dates from the context value or from incoming gRPC metadata.
func RangeFrom(ctx context.Context) (Query, bool) {
	if q, ok := rangeFromValue(ctx); ok && q.HasDates() {
		return q, true
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return Query{}, false
	}
	var q Query
	if vs := md.Get(metaMin); len(vs) > 0 {
		n, err := strconv.ParseInt(vs[0], 10, 32)
		if err != nil {
			return Query{}, false
		}
		q.MinSet = true
		q.MinDate = int32(n)
	}
	if vs := md.Get(metaMax); len(vs) > 0 {
		n, err := strconv.ParseInt(vs[0], 10, 32)
		if err != nil {
			return Query{}, false
		}
		q.MaxSet = true
		q.MaxDate = int32(n)
	}
	return q, q.HasDates()
}

func rangeFromValue(ctx context.Context) (Query, bool) {
	q, ok := ctx.Value(rangeKey{}).(Query)
	return q, ok
}

// Validate rejects a negative bound and a min_date that is after max_date.
// An empty window (min_date == max_date) is valid and deletes nothing.
func Validate(q Query) error {
	if q.MinSet && q.MinDate < 0 {
		return mtproto.ErrMinDateInvalid
	}
	if q.MaxSet && q.MaxDate < 0 {
		return mtproto.ErrMaxDateInvalid
	}
	if q.MinSet && q.MaxSet && q.MinDate > q.MaxDate {
		return mtproto.ErrMinDateInvalid
	}
	return nil
}

// Match is the official date and max_id predicate.
// min_date keeps messages strictly newer than the timestamp.
// max_date keeps messages strictly older than the timestamp.
// max_id <= 0 does not cap the id. A positive max_id keeps id <= max_id.
func Match(id int32, date int64, q Query) bool {
	if q.MaxID > 0 && id > q.MaxID {
		return false
	}
	if q.MinSet && date <= int64(q.MinDate) {
		return false
	}
	if q.MaxSet && (q.MaxDate <= 0 || date >= int64(q.MaxDate)) {
		return false
	}
	return true
}

// PageSelect builds the bounded read. anchorID, when set, is the just_clear
// top message and is not part of the delete page (id < anchor).
func PageSelect(q Query, anchorID, limit int32) Select {
	if limit <= 0 {
		limit = Batch + 1
	}
	sel := Select{Limit: limit}
	if q.MinSet {
		if q.MinDate >= math.MaxInt32 {
			sel.Empty = true
			return sel
		}
		// date2 >= min+1 is date2 > min.
		sel.AfterDate = int64(q.MinDate) + 1
	}
	if q.MaxSet {
		if q.MaxDate <= 0 {
			sel.Empty = true
			return sel
		}
		// date2 < max.
		sel.BeforeDate = int64(q.MaxDate)
	}
	if q.MinSet && q.MaxSet && sel.AfterDate > 0 && sel.BeforeDate > 0 && sel.AfterDate >= sel.BeforeDate {
		sel.Empty = true
		return sel
	}
	sel.BeforeID, sel.BeforeIDSet = upperBound(q.MaxID, anchorID)
	return sel
}

// upperBound is the exclusive id cap (user_message_box_id < bound).
// max_id is inclusive. The anchor is exclusive so just_clear can keep it.
func upperBound(maxID, anchor int32) (int32, bool) {
	set := false
	bound := int32(math.MaxInt32)
	if maxID > 0 && maxID < math.MaxInt32 {
		bound = maxID + 1
		set = true
	}
	if anchor > 0 && (!set || anchor < bound) {
		bound = anchor
		set = true
	}
	if bound == math.MaxInt32 && !set {
		return 0, false
	}
	return bound, set
}

// Page takes a newest-first page of at most limit+1 rows.
// When more than limit rows were returned, offset is the smallest id in
// the batch the caller should pass back as max_id. Otherwise offset is 0.
func Page(rows []Row, limit int) (deleteRows []Row, offset int32) {
	if limit < 1 {
		limit = Batch
	}
	more := len(rows) > limit
	n := len(rows)
	if more {
		n = limit
	}
	deleteRows = append([]Row(nil), rows[:n]...)
	if more && n > 0 {
		offset = deleteRows[n-1].ID
	}
	return deleteRows, offset
}

// PeerRevokeQuery is the window applied on the other participant.
// maxID is the caller's max_id (0 means no id cap). Dates use the same
// newer-than / older-than rules as the caller's page.
func PeerRevokeQuery(maxID int32, justClear bool, dates Query) Query {
	return Query{
		MaxID:     maxID,
		JustClear: justClear,
		MinSet:    dates.MinSet,
		MinDate:   dates.MinDate,
		MaxSet:    dates.MaxSet,
		MaxDate:   dates.MaxDate,
	}
}

// SplitDeleted chooses the ids that get updateDeleteMessages.
// A failed follow-up read must still announce every id the delete statement
// accepted. Otherwise a retry no longer sees those rows and never writes the
// pts update, so the messages stay gone on the server and come back in the
// client that missed the update.
func SplitDeleted(ids []int32, live map[int32]struct{}, confirmErr error) (deleted, failed []int32) {
	if confirmErr != nil {
		return append([]int32(nil), ids...), nil
	}
	for _, id := range ids {
		if _, still := live[id]; still {
			failed = append(failed, id)
			continue
		}
		deleted = append(deleted, id)
	}
	return deleted, failed
}

// IsHistoryClear reports the service message just_clear leaves behind.
func IsHistoryClear(m *mtproto.Message) bool {
	return m.GetPredicateName() == mtproto.Predicate_messageService &&
		m.GetAction().GetPredicateName() == mtproto.Predicate_messageActionHistoryClear
}

// OffsetAfterFailure raises the next max_id so a message that is still
// present is inside the next id <= max_id read. A planned offset of 0
// would tell the client to stop, and the leftover would load again later.
func OffsetAfterFailure(pageOffset int32, failed []int32) int32 {
	if len(failed) == 0 {
		return pageOffset
	}
	top := failed[0]
	for _, id := range failed[1:] {
		if id > top {
			top = id
		}
	}
	if top > pageOffset {
		return top
	}
	return pageOffset
}
