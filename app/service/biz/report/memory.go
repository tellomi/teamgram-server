package report

import (
	"context"
	"encoding/json"
	"sync"
)

type memoryStore struct {
	mu      sync.Mutex
	next    int64
	rows    []*Row
	notices map[string]struct{}
}

// NewMemory returns an empty in-process store.
func NewMemory() Store {
	return &memoryStore{notices: map[string]struct{}{}}
}

func noticeKey(target string, user int64, kind, decision string) string {
	return target + "|" + itoa(user) + "|" + kind + "|" + decision
}

func (m *memoryStore) Insert(ctx context.Context, row *Row) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	row.ID = m.next
	m.rows = append(m.rows, copyRow(row))
	return nil
}

func (m *memoryStore) Get(ctx context.Context, id int64) (*Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.rows {
		if row.ID == id {
			return copyRow(row), nil
		}
	}
	return nil, nil
}

func (m *memoryStore) ListAll(ctx context.Context) ([]*Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Row, 0, len(m.rows))
	for _, row := range m.rows {
		out = append(out, copyRow(row))
	}
	return out, nil
}

func (m *memoryStore) ListByTarget(ctx context.Context, key string) ([]*Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Row
	for _, row := range m.rows {
		if row.TargetKey == key {
			out = append(out, copyRow(row))
		}
	}
	return out, nil
}

func (m *memoryStore) ListPending(ctx context.Context, limit int) ([]*Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Row
	for _, row := range m.rows {
		if row.Status != StatusPending && row.Status != StatusNeedsHuman {
			continue
		}
		out = append(out, copyRow(row))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memoryStore) Recent(ctx context.Context, reporter int64, since int32) (int, int32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int
	var oldest int32
	for _, row := range m.rows {
		if row.Reporter != reporter || row.CreatedAt < since {
			continue
		}
		count++
		if oldest == 0 || row.CreatedAt < oldest {
			oldest = row.CreatedAt
		}
	}
	return count, oldest, nil
}

func (m *memoryStore) UpdateStatus(ctx context.Context, ids []int64, status string, decidedAt int32) error {
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.rows {
		if want[row.ID] {
			row.Status = status
			row.DecidedAt = decidedAt
		}
	}
	return nil
}

func (m *memoryStore) UpdateEvidence(ctx context.Context, id int64, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.rows {
		if row.ID == id {
			row.EvidenceKey = key
			return nil
		}
	}
	return nil
}

func (m *memoryStore) HasLegacy(ctx context.Context, storyID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.rows {
		if row.LegacyStoryID == storyID && storyID != 0 {
			return true, nil
		}
	}
	return false, nil
}

func (m *memoryStore) ClaimNotice(ctx context.Context, targetKey string, userID int64, kind, decision string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := noticeKey(targetKey, userID, kind, decision)
	if _, ok := m.notices[k]; ok {
		return false, nil
	}
	m.notices[k] = struct{}{}
	return true, nil
}

func (m *memoryStore) ReleaseNotice(ctx context.Context, targetKey string, userID int64, kind, decision string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.notices, noticeKey(targetKey, userID, kind, decision))
	return nil
}

// marshalState is the on-disk shape used to prove a reload keeps every row.
func (m *memoryStore) marshalState() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	payload := struct {
		Next int64  `json:"next"`
		Rows []*Row `json:"rows"`
	}{Next: m.next, Rows: m.rows}
	return json.Marshal(payload)
}

func (m *memoryStore) unmarshalState(raw []byte) error {
	var payload struct {
		Next int64  `json:"next"`
		Rows []*Row `json:"rows"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next = payload.Next
	m.rows = nil
	for _, row := range payload.Rows {
		m.rows = append(m.rows, copyRow(row))
	}
	if m.notices == nil {
		m.notices = map[string]struct{}{}
	}
	return nil
}
