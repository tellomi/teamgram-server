package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPlanHistoryDeleteOffsetShrinks(t *testing.T) {
	rows := []historyRow{{BoxID: 5}, {BoxID: 4}, {BoxID: 3}, {BoxID: 2}, {BoxID: 1}}
	plan := planHistoryDelete(5, rows, 2, false)
	if plan.Offset != 3 || len(plan.Delete) != 2 || plan.Delete[0].BoxID != 5 || plan.Read {
		t.Fatalf("first %#v", plan)
	}
	plan = planHistoryDelete(3, rows[2:], 2, false)
	if plan.Offset != 1 || len(plan.Delete) != 2 || plan.Read {
		t.Fatalf("second %#v", plan)
	}
	plan = planHistoryDelete(1, rows[4:], 2, false)
	if plan.Offset != 0 || len(plan.Delete) != 1 || !plan.Read {
		t.Fatalf("last %#v", plan)
	}
}

func TestPlanHistoryDeleteProtectsNewestUntilLastSlice(t *testing.T) {
	rows := []historyRow{{BoxID: 5, Date: 50}, {BoxID: 4}, {BoxID: 3}, {BoxID: 2}, {BoxID: 1}}
	plan := planHistoryDelete(5, rows, 2, true)
	if plan.Offset != 2 || plan.Rewrite != nil || len(plan.Delete) != 2 || plan.Delete[0].BoxID != 4 {
		t.Fatalf("clear slice %#v", plan)
	}
	plan = planHistoryDelete(3, rows[0:3], 2, true)
	if plan.Offset != 0 || plan.Rewrite == nil || plan.Rewrite.BoxID != 5 || len(plan.Delete) != 2 || !plan.Read {
		t.Fatalf("final clear %#v", plan)
	}
	only := planHistoryDelete(1, rows[:1], 2, true)
	if only.Offset != 0 || len(only.Delete) != 0 || only.Rewrite == nil || only.Rewrite.BoxID != 5 {
		t.Fatalf("single %#v", only)
	}
}

func TestHistoryUpdatesPtsMatchesCount(t *testing.T) {
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	plan := historyPlan{
		Delete:  []historyRow{{BoxID: 4}, {BoxID: 3}},
		Rewrite: &historyRow{BoxID: 5, Date: 8},
		Read:    true,
	}
	clear := historyClearMessage(1, peer, *plan.Rewrite)
	ups, n := historyUpdates(peer, plan, clear, 20)
	if n != 4 || len(ups) != 3 {
		t.Fatalf("count %d ups %d", n, len(ups))
	}
	var sum int32
	for _, upd := range ups {
		sum += upd.GetPtsCount()
	}
	if sum != n || ups[len(ups)-1].GetPts_INT32() != 20 {
		t.Fatalf("pts sum %d last %d", sum, ups[len(ups)-1].GetPts_INT32())
	}
	if ups[0].GetPredicateName() != mtproto.Predicate_updateDeleteMessages || len(ups[0].GetMessages()) != 2 || ups[0].GetPtsCount() != 2 {
		t.Fatalf("delete update %#v", ups[0])
	}
	// pts on an update is the end of its range: the next update starts one past it.
	if ups[0].GetPts_INT32()+ups[1].GetPtsCount() != ups[1].GetPts_INT32() {
		t.Fatalf("pts gap delete=%d/%d read=%d/%d", ups[0].GetPts_INT32(), ups[0].GetPtsCount(), ups[1].GetPts_INT32(), ups[1].GetPtsCount())
	}
}

func TestDeleteHistoryDateRangeAndResume(t *testing.T) {
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	st := &memHist{peer: peer}
	for i, date := range []int32{10, 20, 30, 40, 50} {
		st.msgs = append(st.msgs, memMsg{
			historyRow: historyRow{BoxID: int32(i + 1), DataID: int64(100 + i), Date: date},
			live:       true,
			text:       string(rune('a' + i)),
		})
	}
	ctx := context.Background()
	req := historyRequest{
		UserID: 1, AuthKeyID: 7, Peer: peer, Revoke: true,
		MinDate: 20, MaxDate: 40, Batch: 2,
	}
	var offsets []int32
	attempts := 0
	for step := 0; step < 8; step++ {
		attempts++
		call := attempts
		req.BeforeCommit = func() error {
			if call == 2 {
				return errors.New("batch failed")
			}
			return nil
		}
		got, err := deleteHistoryOnce(ctx, st, req)
		if call == 2 {
			if err == nil {
				t.Fatal("failed batch was reported as success")
			}
			if len(st.liveIDs()) != 3 {
				t.Fatalf("failed batch changed rows %v", st.liveIDs())
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		offsets = append(offsets, got.Offset)
		if got.PtsCount <= 0 {
			t.Fatalf("empty pts on a real delete %#v", got)
		}
		if got.Offset == 0 {
			break
		}
	}
	if len(offsets) < 2 || offsets[len(offsets)-1] != 0 {
		t.Fatalf("offsets %v", offsets)
	}
	for i := 1; i < len(offsets); i++ {
		if offsets[i] >= offsets[i-1] {
			t.Fatalf("offset did not shrink %v", offsets)
		}
	}
	live := st.liveIDs()
	if len(live) != 2 || live[0] != 1 || live[1] != 5 {
		t.Fatalf("date range left %v", live)
	}
	if len(st.revoked) != 3 {
		t.Fatalf("revoke ids %v", st.revoked)
	}
	for _, upd := range st.pushed {
		if upd.GetPredicateName() == mtproto.Predicate_updateDeleteMessages {
			for _, id := range upd.GetMessages() {
				if id == 1 || id == 5 {
					t.Fatalf("delete update included a keeper %v", upd.GetMessages())
				}
			}
			if upd.GetPtsCount() != int32(len(upd.GetMessages())) || upd.GetPts_INT32() == 0 {
				t.Fatalf("pts_count %#v", upd)
			}
		}
	}
}

func TestDeleteHistoryRevokeFailureKeepsTheSlice(t *testing.T) {
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	st := &memHist{peer: peer, failRevoke: true}
	st.msgs = []memMsg{{historyRow: historyRow{BoxID: 1, DataID: 8, Date: 1}, live: true}}
	_, err := deleteHistoryOnce(context.Background(), st, historyRequest{
		UserID: 1, Peer: peer, Revoke: true, Batch: 10,
	})
	if err == nil {
		t.Fatal("revoke failure returned success")
	}
	if len(st.liveIDs()) != 1 || st.pts != 0 {
		t.Fatalf("slice changed ids %v pts %d", st.liveIDs(), st.pts)
	}
}

func TestDeleteHistoryJustClearRewritesTheNewest(t *testing.T) {
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	st := &memHist{peer: peer}
	st.msgs = []memMsg{
		{historyRow: historyRow{BoxID: 1, DataID: 1, Date: 1}, live: true, text: "old"},
		{historyRow: historyRow{BoxID: 2, DataID: 2, Date: 2}, live: true, text: "new"},
	}
	var off int32 = 1
	for off > 0 {
		got, err := deleteHistoryOnce(context.Background(), st, historyRequest{
			UserID: 1, Peer: peer, JustClear: true, Batch: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		off = got.Offset
	}
	if len(st.liveIDs()) != 1 || st.liveIDs()[0] != 2 || !st.msgs[1].clear || st.msgs[1].text != "" {
		t.Fatalf("clear result %#v", st.msgs)
	}
	if st.top != 2 {
		t.Fatalf("dialog top %d", st.top)
	}
}

type memMsg struct {
	historyRow
	text  string
	live  bool
	clear bool
}

type memHist struct {
	peer       *mtproto.PeerUtil
	msgs       []memMsg
	pts        int32
	revoked    []int64
	pushed     []*mtproto.Update
	top        int32
	failRevoke bool
}

func (m *memHist) liveIDs() []int32 {
	var ids []int32
	for _, msg := range m.msgs {
		if msg.live {
			ids = append(ids, msg.BoxID)
		}
	}
	return ids
}

func (m *memHist) List(_ context.Context, _ int64, _ *mtproto.PeerUtil, maxID, minDate, maxDate, limit int32) ([]historyRow, int32, error) {
	var rows []historyRow
	for i := len(m.msgs) - 1; i >= 0; i-- {
		msg := m.msgs[i]
		if !msg.live {
			continue
		}
		if maxID > 0 && msg.BoxID > maxID {
			continue
		}
		if minDate > 0 && msg.Date < minDate {
			continue
		}
		if maxDate > 0 && msg.Date > maxDate {
			continue
		}
		rows = append(rows, msg.historyRow)
	}
	total := int32(len(rows))
	if limit > 0 && int32(len(rows)) > limit {
		rows = rows[:limit]
	}
	return rows, total, nil
}

func (m *memHist) TopID(context.Context, int64, *mtproto.PeerUtil) (int32, error) {
	var top int32
	for _, msg := range m.msgs {
		if msg.live && msg.BoxID > top {
			top = msg.BoxID
		}
	}
	return top, nil
}

func (m *memHist) Revoke(_ context.Context, _ int64, _ *mtproto.PeerUtil, dataIDs []int64) error {
	if m.failRevoke {
		return errors.New("inbox down")
	}
	m.revoked = append(m.revoked, dataIDs...)
	return nil
}

func (m *memHist) Delete(_ context.Context, _ int64, ids []int32) error {
	drop := map[int32]bool{}
	for _, id := range ids {
		drop[id] = true
	}
	for i := range m.msgs {
		if drop[m.msgs[i].BoxID] {
			m.msgs[i].live = false
			m.msgs[i].text = ""
		}
	}
	return nil
}

func (m *memHist) Rewrite(_ context.Context, userID int64, peer *mtproto.PeerUtil, row historyRow) (*mtproto.Message, error) {
	for i := range m.msgs {
		if m.msgs[i].BoxID == row.BoxID {
			m.msgs[i].text = ""
			m.msgs[i].clear = true
		}
	}
	return historyClearMessage(userID, peer, row), nil
}

func (m *memHist) Touch(context.Context, int64, *mtproto.PeerUtil) error {
	top, _ := m.TopID(context.Background(), 0, nil)
	m.top = top
	return nil
}

func (m *memHist) CurrentPts(context.Context, int64) int32 { return m.pts }

func (m *memHist) AllocPts(_ context.Context, _ int64, n int32) (int32, error) {
	m.pts += n
	return m.pts, nil
}

func (m *memHist) Persist(context.Context, int64, int32, int32, *mtproto.Update) error { return nil }

func (m *memHist) Push(_ context.Context, _ int64, _ int64, upds []*mtproto.Update) error {
	m.pushed = append(m.pushed, upds...)
	return nil
}
