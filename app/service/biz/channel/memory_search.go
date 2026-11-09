package channel

import (
	"context"
	"sort"
)

func (s *memoryStore) SearchPosts(ctx context.Context, self int64, q PostQuery) ([]*StoredMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if q.Limit <= 0 {
		q.Limit = 20
	}
	var hits []*StoredMessage
	for id, ch := range s.channels {
		p := s.parts[id][self]
		if !postSearchVisible(ch, p) {
			continue
		}
		for _, stored := range s.messages[id] {
			if stored == nil || stored.Deleted {
				continue
			}
			if !olderPost(stored.Date, stored.ChannelID, stored.ID, q) {
				continue
			}
			if !containsFold(messagePlain(stored.Data), q.Needle) {
				continue
			}
			hits = append(hits, stored.clone())
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Date != hits[j].Date {
			return hits[i].Date > hits[j].Date
		}
		if hits[i].ChannelID != hits[j].ChannelID {
			return hits[i].ChannelID > hits[j].ChannelID
		}
		return hits[i].ID > hits[j].ID
	})
	if int32(len(hits)) > q.Limit {
		hits = hits[:q.Limit]
	}
	return hits, nil
}

func (s *memoryStore) SearchPublic(ctx context.Context, self int64, q string, limit int32) ([]*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Channel
	for id, ch := range s.channels {
		if channelHidden(ch) || ch.Username == "" || viewerBlocked(s.parts[id][self]) {
			continue
		}
		if !containsFold(ch.Title, q) && !containsFold(ch.Username, q) {
			continue
		}
		out = append(out, ch.clone())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ParticipantsCount != out[j].ParticipantsCount {
			return out[i].ParticipantsCount > out[j].ParticipantsCount
		}
		return out[i].ID < out[j].ID
	})
	if limit > 0 && int32(len(out)) > limit {
		out = out[:limit]
	}
	return out, nil
}
