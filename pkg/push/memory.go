package push

import (
	"context"
	"sync"
)

// MemoryStore is an in-process Store for tests and for processes that have no MySQL.
type MemoryStore struct {
	mu      sync.Mutex
	devices []Device
}

func (s *MemoryStore) Upsert(_ context.Context, d Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]Device, 0, len(s.devices)+1)
	for _, old := range s.devices {
		if old.UserID == d.UserID && old.TokenType == d.TokenType && old.Token == d.Token && old.AuthKeyID != d.AuthKeyID {
			continue
		}
		if old.AuthKeyID == d.AuthKeyID && old.UserID == d.UserID && old.TokenType == d.TokenType {
			continue
		}
		kept = append(kept, old)
	}
	s.devices = append(kept, cloneDevice(d))
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, userID int64, tokenType int32, token string, otherUserIDs []int64) error {
	ids := map[int64]struct{}{userID: {}}
	for _, id := range otherUserIDs {
		ids[id] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]Device, 0, len(s.devices))
	for _, old := range s.devices {
		if _, ok := ids[old.UserID]; ok && old.TokenType == tokenType && old.Token == token {
			continue
		}
		kept = append(kept, old)
	}
	s.devices = kept
	return nil
}

func (s *MemoryStore) ListByUser(_ context.Context, userID int64) ([]Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Device
	for _, d := range s.devices {
		if d.UserID == userID {
			out = append(out, cloneDevice(d))
		}
	}
	return out, nil
}

func cloneDevice(d Device) Device {
	d.Secret = append([]byte(nil), d.Secret...)
	d.OtherUIDs = append([]int64(nil), d.OtherUIDs...)
	return d
}
