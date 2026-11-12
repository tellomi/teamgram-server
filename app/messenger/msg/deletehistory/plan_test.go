package deletehistory

import (
	"context"
	"math"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/metadata"
)

func TestMatchOfficialDateAndMaxID(t *testing.T) {
	q := Query{MaxID: 50, MinSet: true, MinDate: 100, MaxSet: true, MaxDate: 200}
	if !Match(50, 150, q) {
		t.Fatal("id 50 date 150 is inside the window")
	}
	if Match(51, 150, q) {
		t.Fatal("id above max_id must stay")
	}
	if Match(10, 100, q) {
		t.Fatal("min_date is newer-than, so the boundary stays")
	}
	if Match(10, 200, q) {
		t.Fatal("max_date is older-than, so the boundary stays")
	}
	if !Match(10, 101, q) || !Match(10, 199, q) {
		t.Fatal("dates strictly inside the window must match")
	}
	open := Query{}
	if !Match(math.MaxInt32, 0, open) {
		t.Fatal("an unset max_id and unset dates match every live message")
	}
	future := Query{MinSet: true, MinDate: 1_000_000}
	if Match(1, 10, future) {
		t.Fatal("a message older than min_date must stay")
	}
}

func TestValidateDates(t *testing.T) {
	if err := Validate(Query{MinSet: true, MinDate: -1}); err != mtproto.ErrMinDateInvalid {
		t.Fatalf("negative min: %v", err)
	}
	if err := Validate(Query{MaxSet: true, MaxDate: -5}); err != mtproto.ErrMaxDateInvalid {
		t.Fatalf("negative max: %v", err)
	}
	if err := Validate(Query{MinSet: true, MinDate: 20, MaxSet: true, MaxDate: 10}); err != mtproto.ErrMinDateInvalid {
		t.Fatalf("min after max: %v", err)
	}
	if err := Validate(Query{MinSet: true, MinDate: 10, MaxSet: true, MaxDate: 10}); err != nil {
		t.Fatalf("equal bounds are an empty window, not an error: %v", err)
	}
}

func TestPageSelectBounds(t *testing.T) {
	sel := PageSelect(Query{MaxID: 50, MinSet: true, MinDate: 100, MaxSet: true, MaxDate: 200}, 40, Batch+1)
	if sel.Empty || !sel.BeforeIDSet || sel.BeforeID != 40 {
		t.Fatalf("anchor must be the exclusive id cap: %+v", sel)
	}
	if sel.AfterDate != 101 || sel.BeforeDate != 200 || sel.Limit != Batch+1 {
		t.Fatalf("date bounds: %+v", sel)
	}

	wide := PageSelect(Query{MaxID: math.MaxInt32}, 0, 0)
	if wide.Empty || wide.BeforeIDSet || wide.AfterDate != 0 || wide.BeforeDate != 0 || wide.Limit != Batch+1 {
		t.Fatalf("max int max_id is unbounded: %+v", wide)
	}

	none := PageSelect(Query{MinSet: true, MinDate: 10, MaxSet: true, MaxDate: 10}, 0, Batch+1)
	if !none.Empty {
		t.Fatalf("equal dates match nothing: %+v", none)
	}
	past := PageSelect(Query{MaxSet: true, MaxDate: 0}, 0, Batch+1)
	if !past.Empty {
		t.Fatalf("older than 0 matches nothing: %+v", past)
	}

	capped := PageSelect(Query{MaxID: 50}, 80, Batch+1)
	if !capped.BeforeIDSet || capped.BeforeID != 51 {
		t.Fatalf("max_id is inclusive, so the exclusive cap is max_id+1: %+v", capped)
	}
}

func TestPageOffsetKeepsTheClientLooping(t *testing.T) {
	rows := make([]Row, Batch+1)
	for i := range rows {
		rows[i] = Row{ID: int32(500 - i)}
	}
	got, offset := Page(rows, Batch)
	if len(got) != Batch || got[0].ID != 500 || got[Batch-1].ID != 401 {
		t.Fatalf("page ids %d..%d len %d", got[0].ID, got[len(got)-1].ID, len(got))
	}
	if offset != 401 {
		t.Fatalf("offset %d, want the oldest id in the page so the next max_id continues downward", offset)
	}

	short, offset := Page(rows[:3], Batch)
	if len(short) != 3 || offset != 0 {
		t.Fatalf("a short page is finished: len %d offset %d", len(short), offset)
	}
}

func TestOffsetAfterFailureDoesNotReportDone(t *testing.T) {
	if got := OffsetAfterFailure(0, nil); got != 0 {
		t.Fatalf("no failure and no remainder: %d", got)
	}
	if got := OffsetAfterFailure(0, []int32{12, 40, 7}); got != 40 {
		t.Fatalf("failures with a finished page must be retried via max_id %d", got)
	}
	// The planned offset is the oldest id. A newer id that did not delete
	// is above it, so the next max_id has to move up to include that id.
	if got := OffsetAfterFailure(10, []int32{10, 25}); got != 25 {
		t.Fatalf("failed newer id must stay inside the next max_id, got %d", got)
	}
}

func TestRangeSurvivesGRPCMetadata(t *testing.T) {
	ctx := WithRange(context.Background(), Query{MinSet: true, MinDate: 10, MaxSet: true, MaxDate: 0})
	out := AppendOutgoing(ctx)
	md, ok := metadata.FromOutgoingContext(out)
	if !ok {
		t.Fatal("no outgoing metadata")
	}
	in := metadata.NewIncomingContext(context.Background(), md)
	q, ok := RangeFrom(in)
	if !ok || !q.MinSet || q.MinDate != 10 || !q.MaxSet || q.MaxDate != 0 {
		t.Fatalf("metadata range %+v ok %v", q, ok)
	}
	if _, ok := RangeFrom(context.Background()); ok {
		t.Fatal("empty context has no date range")
	}
}
