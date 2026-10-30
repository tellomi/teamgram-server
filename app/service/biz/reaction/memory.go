package reaction

import (
	"context"
	"sync"
	"time"
)

type memRow struct {
	Row
	id int64
}

type memoryStore struct {
	mu    sync.Mutex
	next  int64
	rows  map[Key][]memRow
	boxes map[boxKey]Key
	pts   []ptsRow
}

type boxKey struct {
	UserID   int64
	PeerType int32
	PeerID   int64
	MsgID    int32
}

type ptsRow struct {
	UserID     int64
	Pts        int32
	PtsCount   int32
	UpdateType int32
	JSON       string
	Date       int64
}

// NewMemory returns an empty store. Each channel service gets its own.
func NewMemory() Store {
	return &memoryStore{
		rows:  map[Key][]memRow{},
		boxes: map[boxKey]Key{},
	}
}

func (s *memoryStore) Replace(ctx context.Context, key Key, actor int64, emoticons []string, big bool, boxes []Box) ([]Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.rows[key]
	kept := make([]memRow, 0, len(prev)+len(emoticons))
	for _, r := range prev {
		if r.UserID != actor {
			kept = append(kept, r)
		}
	}
	now := int32(time.Now().Unix())
	seen := map[string]bool{}
	for _, emo := range emoticons {
		if emo == "" || seen[emo] {
			continue
		}
		seen[emo] = true
		s.next++
		kept = append(kept, memRow{Row: Row{UserID: actor, Emoticon: emo, Big: big, Date: now}, id: s.next})
	}
	if len(kept) == 0 {
		delete(s.rows, key)
	} else {
		s.rows[key] = kept
	}
	for _, b := range boxes {
		if b.UserID == 0 || b.MsgID == 0 {
			continue
		}
		s.boxes[boxKey{b.UserID, b.PeerType, b.PeerID, b.MsgID}] = key
	}
	return cloneRows(s.rows[key]), nil
}

func (s *memoryStore) List(ctx context.Context, key Key) ([]Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneRows(s.rows[key]), nil
}

func (s *memoryStore) ListMany(ctx context.Context, keys []Key) (map[Key][]Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[Key][]Row{}
	for _, k := range keys {
		if rows := s.rows[k]; len(rows) > 0 {
			out[k] = cloneRows(rows)
		}
	}
	return out, nil
}

func (s *memoryStore) KeyForBox(ctx context.Context, userID int64, peerType int32, peerID int64, msgID int32) (Key, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.boxes[boxKey{userID, peerType, peerID, msgID}]
	return k, ok, nil
}

func (s *memoryStore) SavePts(ctx context.Context, userID int64, pts, ptsCount, updateType int32, updateJSON string, date int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pts = append(s.pts, ptsRow{userID, pts, ptsCount, updateType, updateJSON, date})
	return nil
}

func cloneRows(in []memRow) []Row {
	if len(in) == 0 {
		return nil
	}
	out := make([]Row, len(in))
	for i, r := range in {
		out[i] = r.Row
	}
	return out
}
