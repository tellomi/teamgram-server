package discoverpush

import (
	"context"
	"sync"
)

// MemorySubjects is a Subjects for tests.
type MemorySubjects struct {
	mu    sync.Mutex
	users map[string]int64
}

// NewMemorySubjects returns an empty map.
func NewMemorySubjects() *MemorySubjects {
	return &MemorySubjects{users: map[string]int64{}}
}

// Save records sub → userID. sub is 16 raw bytes.
func (m *MemorySubjects) Save(userID int64, sub []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[subKey(sub)] = userID
}

// Users resolves subs.
func (m *MemorySubjects) Users(_ context.Context, subs [][]byte) (map[string]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]int64{}
	for _, sub := range subs {
		if id, ok := m.users[subKey(sub)]; ok {
			out[subKey(sub)] = id
		}
	}
	return out, nil
}

// MemoryEvents is an Events for tests.
type MemoryEvents struct {
	mu   sync.Mutex
	seen map[string]int64
}

// NewMemoryEvents returns an empty claim set.
func NewMemoryEvents() *MemoryEvents {
	return &MemoryEvents{seen: map[string]int64{}}
}

// Claim inserts id. The second call is not fresh.
func (m *MemoryEvents) Claim(_ context.Context, id string, userID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.seen[id]; ok {
		return false, nil
	}
	m.seen[id] = userID
	return true, nil
}

// Release drops a claim so a failed push can be retried.
func (m *MemoryEvents) Release(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.seen, id)
	return nil
}
