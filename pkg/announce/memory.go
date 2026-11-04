package announce

import (
	"context"
	"strconv"
	"sync"
)

// Memory is the in-process directory and claim log used by tests.
type Memory struct {
	mu       sync.Mutex
	ids      []int64
	users    map[string]struct{}
	channels map[string]struct{}
	failPage bool
}

// NewMemory returns an empty store.
func NewMemory(ids []int64) *Memory {
	return &Memory{
		ids:      append([]int64(nil), ids...),
		users:    map[string]struct{}{},
		channels: map[string]struct{}{},
	}
}

// Page implements Directory. Official accounts are skipped.
func (m *Memory) Page(_ context.Context, after int64, limit int) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failPage {
		return nil, ErrUnavailable
	}
	var out []int64
	for _, id := range m.ids {
		if id <= after || Official(id) {
			continue
		}
		out = append(out, id)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func claimKey(eventID string, userID int64) string {
	return eventID + "\x00" + strconv.FormatInt(userID, 10)
}

// ClaimUser implements Claims.
func (m *Memory) ClaimUser(_ context.Context, eventID string, userID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := claimKey(eventID, userID)
	if _, ok := m.users[k]; ok {
		return false, nil
	}
	m.users[k] = struct{}{}
	return true, nil
}

// ReleaseUser implements Claims.
func (m *Memory) ReleaseUser(_ context.Context, eventID string, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.users, claimKey(eventID, userID))
	return nil
}

// ClaimChannel implements Claims.
func (m *Memory) ClaimChannel(_ context.Context, eventID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.channels[eventID]; ok {
		return false, nil
	}
	m.channels[eventID] = struct{}{}
	return true, nil
}

// ReleaseChannel implements Claims.
func (m *Memory) ReleaseChannel(_ context.Context, eventID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.channels, eventID)
	return nil
}
