package channel

import "sort"

func normalizeLimit(limit int32) int32 {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// selectHistory returns a page of non-deleted messages and the total live count.
// Messages are newest first. offset_id selects messages older than that id;
// add_offset shifts that window (negative add_offset reaches newer messages).
func selectHistory(all []*StoredMessage, q HistoryQuery) (page []*StoredMessage, total int32) {
	live := make([]*StoredMessage, 0, len(all))
	for _, m := range all {
		if m == nil || m.Deleted {
			continue
		}
		if q.MaxID > 0 && m.ID >= q.MaxID {
			continue
		}
		if q.MinID > 0 && m.ID <= q.MinID {
			continue
		}
		live = append(live, m)
	}
	total = int32(len(live))
	sort.Slice(live, func(i, j int) bool { return live[i].ID > live[j].ID })

	pivot := 0
	if q.OffsetID > 0 {
		pivot = len(live)
		for i, m := range live {
			if m.ID < q.OffsetID {
				pivot = i
				break
			}
		}
	}
	if q.OffsetDate > 0 {
		found := false
		for i := pivot; i < len(live); i++ {
			if live[i].Date < q.OffsetDate {
				pivot = i
				found = true
				break
			}
		}
		if !found {
			pivot = len(live)
		}
	}
	pivot += int(q.AddOffset)
	if pivot < 0 {
		pivot = 0
	}
	if pivot > len(live) {
		pivot = len(live)
	}
	limit := int(normalizeLimit(q.Limit))
	end := pivot + limit
	if end > len(live) {
		end = len(live)
	}
	return live[pivot:end], total
}
