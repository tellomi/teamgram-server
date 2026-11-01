package book

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"

	"github.com/teamgram/proto/mtproto"
)

// Story blocklist.
// https://core.telegram.org/api/block
//
// contacts.block, contacts.unblock, contacts.getBlocked, and contacts.setBlocked
// edit this list when my_stories_from is set. The main blocklist stays in the
// user service. A peer on this list cannot view the owner's stories.
// userFull.blocked_my_stories_from is read from here.

const storyKeyPrefix = "contacts_story_block_v1:"

// StoryRow is one blocked peer and the unix time it was blocked.
type StoryRow struct {
	Peer Peer
	Date int32
}

type storyRec struct {
	Rows []storyRow `json:"rows,omitempty"`
}

type storyRow struct {
	Type int32 `json:"type"`
	ID   int64 `json:"id"`
	Date int32 `json:"date"`
}

// StoryAdd puts peer on the owner's story blocklist. added is false when it
// was already there.
func (b *Book) StoryAdd(ctx context.Context, owner int64, peer Peer, date int32) (added bool, err error) {
	if peer.ID == 0 {
		return false, nil
	}
	rec := b.loadStories(ctx, owner)
	for _, row := range rec.Rows {
		if row.Type == peer.Type && row.ID == peer.ID {
			return false, nil
		}
	}
	rec.Rows = append(rec.Rows, storyRow{Type: peer.Type, ID: peer.ID, Date: date})
	return true, b.saveStories(ctx, owner, rec)
}

// StoryRemove drops peer from the story blocklist.
func (b *Book) StoryRemove(ctx context.Context, owner int64, peer Peer) (removed bool, err error) {
	rec := b.loadStories(ctx, owner)
	kept := rec.Rows[:0]
	for _, row := range rec.Rows {
		if row.Type == peer.Type && row.ID == peer.ID {
			removed = true
			continue
		}
		kept = append(kept, row)
	}
	if !removed {
		return false, nil
	}
	rec.Rows = kept
	return true, b.saveStories(ctx, owner, rec)
}

// ReplaceStories sets the story blocklist to peers and returns who was added
// and who was removed. Dates of peers that stay are kept.
func (b *Book) ReplaceStories(ctx context.Context, owner int64, peers []Peer, date int32) (added, removed []Peer, err error) {
	rec := b.loadStories(ctx, owner)
	prev := map[string]storyRow{}
	for _, row := range rec.Rows {
		prev[Peer{Type: row.Type, ID: row.ID}.key()] = row
	}
	next := map[string]Peer{}
	var rows []storyRow
	for _, peer := range peers {
		if peer.ID == 0 {
			continue
		}
		k := peer.key()
		if _, dup := next[k]; dup {
			continue
		}
		next[k] = peer
		row, ok := prev[k]
		if !ok {
			row = storyRow{Type: peer.Type, ID: peer.ID, Date: date}
			added = append(added, peer)
		}
		rows = append(rows, row)
	}
	for k, row := range prev {
		if _, ok := next[k]; !ok {
			removed = append(removed, Peer{Type: row.Type, ID: row.ID})
		}
	}
	rec.Rows = rows
	return added, removed, b.saveStories(ctx, owner, rec)
}

// StoryList returns the blocklist, newest first.
func (b *Book) StoryList(ctx context.Context, owner int64) []StoryRow {
	rec := b.loadStories(ctx, owner)
	out := make([]StoryRow, 0, len(rec.Rows))
	for _, row := range rec.Rows {
		out = append(out, StoryRow{Peer: Peer{Type: row.Type, ID: row.ID}, Date: row.Date})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].Peer.ID < out[j].Peer.ID
	})
	return out
}

// StoryBlocked reports whether owner has blocked peer from their stories.
func (b *Book) StoryBlocked(ctx context.Context, owner, peer int64) bool {
	if b == nil || owner == 0 || peer == 0 || owner == peer {
		return false
	}
	for _, row := range b.loadStories(ctx, owner).Rows {
		if row.Type == mtproto.PEER_USER && row.ID == peer {
			return true
		}
	}
	return false
}

func (b *Book) loadStories(ctx context.Context, owner int64) storyRec {
	raw, err := b.kv.GetCtx(ctx, storyKeyPrefix+strconv.FormatInt(owner, 10))
	if err != nil || raw == "" {
		return storyRec{}
	}
	var rec storyRec
	if json.Unmarshal([]byte(raw), &rec) != nil {
		return storyRec{}
	}
	return rec
}

func (b *Book) saveStories(ctx context.Context, owner int64, rec storyRec) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return b.kv.SetCtx(ctx, storyKeyPrefix+strconv.FormatInt(owner, 10), string(raw))
}
