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

func TestRevokePeerCapClearsPeerAfterCallerAlreadyDeleted(t *testing.T) {
	if got := RevokePeerCap(19, 44, false); got != 44 {
		t.Fatalf("deleted caller row still maps, got %d", got)
	}
	if got := RevokePeerCap(19, 0, false); got != 0 {
		t.Fatalf("no caller row and no live messages must clear the peer, got %d", got)
	}
	if got := RevokePeerCap(19, 0, true); got != 19 {
		t.Fatalf("live messages keep the caller cap when the map misses, got %d", got)
	}
	if got := RevokePeerCap(0, 0, false); got != 0 {
		t.Fatalf("uncapped stays uncapped, got %d", got)
	}
}

func TestPeerRevokeMaxIDUsesTheOtherBox(t *testing.T) {
	// A's last box id is 19. The same message is box 44 on B.
	if got := PeerRevokeMaxID(19, 44); got != 44 {
		t.Fatalf("peer max_id %d, want B's box 44", got)
	}
	if got := PeerRevokeMaxID(0, 44); got != 0 {
		t.Fatalf("max_id 0 stays uncapped, got %d", got)
	}
	if got := PeerRevokeMaxID(math.MaxInt32, 44); got != math.MaxInt32 {
		t.Fatalf("max int stays uncapped, got %d", got)
	}
	if got := PeerRevokeMaxID(19, 0); got != 19 {
		t.Fatalf("missing peer row keeps the caller id, got %d", got)
	}
	// A partial clear maps the included message, not every newer peer row.
	if got := PeerRevokeMaxID(20, 120); got != 120 {
		t.Fatalf("partial peer max_id %d", got)
	}
}

func TestCallerLastMaxIDClearsPeerOwnLastAsHistoryClear(t *testing.T) {
	// A boxes 16..19, B boxes 41..44. 19 and 44 are one message: B's own last
	// send. just_clear keeps that message and turns it into historyClear.
	const callerLast int32 = 19
	const peerLast int32 = 44
	peerRows := []Row{{ID: 44, Date: 4}, {ID: 43, Date: 3}, {ID: 42, Date: 2}, {ID: 41, Date: 1}}

	peerMax := PeerRevokeMaxID(callerLast, peerLast)
	q := PeerRevokeQuery(peerMax, true, Query{})
	anchor := newestSelected(peerRows, PageSelect(q, 0, 1))
	if anchor != peerLast {
		t.Fatalf("historyClear anchor %d, want B's last message %d", anchor, peerLast)
	}
	del := PageSelect(q, anchor, Batch+1)
	if !del.BeforeIDSet || del.BeforeID != peerLast {
		t.Fatalf("delete bound %+v, want id < %d so the last message stays and becomes historyClear", del, peerLast)
	}
	var deleted []int32
	for _, row := range peerRows {
		if row.ID < del.BeforeID {
			deleted = append(deleted, row.ID)
		}
	}
	if len(deleted) != 3 || deleted[0] != 43 || deleted[2] != 41 {
		t.Fatalf("deleted %v, want 43, 42, 41", deleted)
	}

	// The bug: A's 19 applied to B matches nothing. B's last message stays
	// a normal message and no historyClear anchor is chosen.
	rawTop := PageSelect(PeerRevokeQuery(callerLast, true, Query{}), 0, 1)
	if got := newestSelected(peerRows, rawTop); got != 0 {
		t.Fatalf("unmapped caller max_id selected peer box %d", got)
	}
}

func newestSelected(rows []Row, sel Select) int32 {
	if sel.Empty {
		return 0
	}
	for _, row := range rows {
		if sel.BeforeIDSet && row.ID >= sel.BeforeID {
			continue
		}
		if sel.AfterDate > 0 && row.Date < sel.AfterDate {
			continue
		}
		if sel.BeforeDate > 0 && row.Date >= sel.BeforeDate {
			continue
		}
		return row.ID
	}
	return 0
}

func TestPeerWindowDoesNotDeleteOutsideMaxIDOrDates(t *testing.T) {
	q := PeerRevokeQuery(50, true, Query{MinSet: true, MinDate: 100, MaxSet: true, MaxDate: 200})
	rows := []Row{
		{ID: 60, Date: 150},
		{ID: 40, Date: 150},
		{ID: 30, Date: 50},
		{ID: 20, Date: 180},
	}
	var inside []Row
	for _, row := range rows {
		if Match(row.ID, row.Date, q) {
			inside = append(inside, row)
		}
	}
	if len(inside) != 2 || inside[0].ID != 40 || inside[1].ID != 20 {
		t.Fatalf("window %v", inside)
	}
	// The anchor is the newest match, not a newer message the caller did not include.
	page, offset := Page(inside[1:], Batch)
	if len(page) != 1 || page[0].ID != 20 || offset != 0 {
		t.Fatalf("page=%v offset=%d", page, offset)
	}
	sel := PageSelect(q, inside[0].ID, Batch+1)
	if !sel.BeforeIDSet || sel.BeforeID != 40 || sel.AfterDate != 101 || sel.BeforeDate != 200 {
		t.Fatalf("peer select %+v", sel)
	}
}

func TestSplitDeletedAnnouncesWhenConfirmFails(t *testing.T) {
	deleted, failed := SplitDeleted([]int32{3, 2, 1}, map[int32]struct{}{2: {}}, nil)
	if len(deleted) != 2 || deleted[0] != 3 || deleted[1] != 1 || len(failed) != 1 || failed[0] != 2 {
		t.Fatalf("deleted %v failed %v", deleted, failed)
	}
	deleted, failed = SplitDeleted([]int32{3, 2, 1}, nil, context.DeadlineExceeded)
	if len(failed) != 0 || len(deleted) != 3 {
		t.Fatalf("confirm failure must still announce the batch: deleted %v failed %v", deleted, failed)
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
