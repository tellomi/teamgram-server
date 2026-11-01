// Package saved is the pure contract for saved-message dialogs and monoforum
// topic lists. Handlers apply it; tests cover it without MySQL.
//
// https://core.telegram.org/api/saved-messages
// https://core.telegram.org/method/messages.getSavedDialogs
// https://core.telegram.org/method/messages.deleteSavedHistory
package saved

import "github.com/teamgram/proto/mtproto"

const (
	// PageCap is the largest page getSavedDialogs returns. Desktop asks for
	// 20, then 100. https://core.telegram.org/method/messages.getSavedDialogs
	PageCap int32 = 100
	// HistoryCap matches messages.getHistory: one page is at most 100.
	HistoryCap int32 = 100
	// DeleteBatch is how many saved messages one deleteSavedHistory call removes.
	// A full batch sets affectedHistory.offset so the client calls again.
	DeleteBatch int32 = 100
	// ByIDCap bounds messages.getSavedDialogsByID. Clients refresh stale
	// topics in groups of 100.
	ByIDCap = 200
)

// Row is one saved dialog used for the cache hash.
type Row struct {
	Pinned   bool
	PeerType int32
	PeerID   int64
	Top      int32
}

// ParentSpecified reports whether parent_peer was sent. Absent and
// inputPeerEmpty both mean "my Saved Messages", not a monoforum.
func ParentSpecified(p *mtproto.InputPeer) bool {
	if p == nil {
		return false
	}
	switch p.PredicateName {
	case "", mtproto.Predicate_inputPeerEmpty:
		return false
	default:
		return true
	}
}

// SavedPeer accepts the peers a saved dialog can name. Empty is rejected.
// https://core.telegram.org/constructor/savedDialog
func SavedPeer(p *mtproto.PeerUtil) bool {
	if p == nil {
		return false
	}
	switch p.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		return p.PeerType == mtproto.PEER_SELF || p.PeerId != 0
	default:
		return false
	}
}

// NormalizeLimit clamps a client limit. Zero becomes the page cap.
func NormalizeLimit(limit, cap int32) int32 {
	if limit <= 0 || limit > cap {
		return cap
	}
	return limit
}

// FullList is messages.savedDialogs (no more pages). A short page, including
// an empty one, is the end. A full page is a slice so the client asks again.
func FullList(n, limit int32) bool {
	return n < limit
}

// Hash is the offsets hash of pinned, peer dialog id, and top_message.
// hash 0 never matches, so a client that does not cache still receives the list.
// https://core.telegram.org/api/offsets
func Hash(rows []Row) int64 {
	var h int64
	for _, row := range rows {
		pinned := int64(0)
		if row.Pinned {
			pinned = 1
		}
		h = mix(h, pinned)
		h = mix(h, mtproto.MakePeerDialogId(row.PeerType, row.PeerID))
		h = mix(h, int64(row.Top))
	}
	return h
}

// NotModified is messages.savedDialogsNotModified when the client hash matches.
func NotModified(hash int64, rows []Row) bool {
	return hash != 0 && hash == Hash(rows)
}

// HashIDs is the same mix over message ids, used by getSavedHistory.
func HashIDs(ids []int32) int64 {
	var h int64
	for _, id := range ids {
		h = mix(h, int64(id))
	}
	return h
}

func mix(hash, id int64) int64 {
	hash ^= id >> 21
	hash ^= id << 35
	hash ^= id >> 4
	return hash + id
}

// InDeleteWindow matches messages.deleteSavedHistory: id <= max_id, optional
// min_date (newer than or equal) and max_date (older than or equal).
// max_id <= 0 deletes nothing.
func InDeleteWindow(id, date, maxID, minDate, maxDate int32) bool {
	if maxID <= 0 || id <= 0 || id > maxID {
		return false
	}
	if minDate > 0 && date < minDate {
		return false
	}
	if maxDate > 0 && date > maxDate {
		return false
	}
	return true
}
