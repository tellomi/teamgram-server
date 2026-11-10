package channel

import "testing"

func liveMessages(n int) []*StoredMessage {
	out := make([]*StoredMessage, 0, n+1)
	for id := 1; id <= n; id++ {
		out = append(out, &StoredMessage{ID: int32(id), Date: int32(id)})
	}
	out = append(out, &StoredMessage{ID: int32(n + 1), Date: int32(n + 1), Deleted: true})
	return out
}

func TestSelectHistoryAllows100(t *testing.T) {
	all := liveMessages(120)
	page, total := selectHistory(all, HistoryQuery{Limit: 100})
	if total != 120 || len(page) != 100 {
		t.Fatalf("total %d page %d", total, len(page))
	}
	if page[0].ID != 120 || page[99].ID != 21 {
		t.Fatalf("window %d..%d", page[0].ID, page[99].ID)
	}
	// A client that asks for more than 100 still gets 100, not 50 and not 20.
	capped, _ := selectHistory(all, HistoryQuery{Limit: 1000})
	if len(capped) != 100 || capped[0].ID != 120 {
		t.Fatalf("capped %d", len(capped))
	}
}

func TestSelectHistoryOffsetAddMinMax(t *testing.T) {
	all := liveMessages(120)
	page, _ := selectHistory(all, HistoryQuery{Limit: 10, OffsetID: 50})
	if len(page) != 10 || page[0].ID != 49 || page[9].ID != 40 {
		t.Fatalf("offset %v", ids(page))
	}
	page, _ = selectHistory(all, HistoryQuery{Limit: 10, OffsetID: 50, AddOffset: 5})
	if len(page) != 10 || page[0].ID != 44 || page[9].ID != 35 {
		t.Fatalf("add_offset %v", ids(page))
	}
	page, _ = selectHistory(all, HistoryQuery{Limit: 100, MinID: 100})
	if len(page) != 20 || page[0].ID != 120 || page[len(page)-1].ID != 101 {
		t.Fatalf("min_id %v", ids(page))
	}
	page, _ = selectHistory(all, HistoryQuery{Limit: 100, MaxID: 30})
	if len(page) != 29 || page[0].ID != 29 || page[len(page)-1].ID != 1 {
		t.Fatalf("max_id %v", ids(page))
	}
	// offset_date walks to the first message older than that date, then add_offset.
	page, _ = selectHistory(all, HistoryQuery{Limit: 3, OffsetDate: 40, AddOffset: 1})
	if len(page) != 3 || page[0].ID != 38 {
		t.Fatalf("offset_date %v", ids(page))
	}
}

func ids(page []*StoredMessage) []int32 {
	out := make([]int32, len(page))
	for i, m := range page {
		out[i] = m.ID
	}
	return out
}
