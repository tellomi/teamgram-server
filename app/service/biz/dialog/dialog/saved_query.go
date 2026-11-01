package dialog

// Internal DialogGetSavedDialogs limits. The MTProto handlers are the only
// callers. A client limit is never negative.
const (
	// SavedLookupLimit returns the one saved dialog in offset_peer, or an
	// empty list when that peer has no row.
	SavedLookupLimit int32 = -1
	// SavedTouchLimit rewrites one saved dialog after deleteSavedHistory.
	// offset_peer is the peer. offset_id is the new top_message. offset_id 0
	// marks the row deleted.
	SavedTouchLimit int32 = -2
)
