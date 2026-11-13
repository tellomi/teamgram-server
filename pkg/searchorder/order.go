// Package searchorder is the messages.searchGlobal page order.
//
// Newest first: date descending, peer type ascending, peer id ascending,
// message id descending. Both the user-message SQL and the channel SQL use
// this order. A later page is the rows strictly after the cursor in this
// order. A channel message id is only compared with another channel message
// id, and a user box id only with another box id on the same peer.
package searchorder

// Key is one message in the global order.
type Key struct {
	Date     int32
	PeerType int32
	PeerID   int64
	MsgID    int32
}

// OlderThan reports that k sorts after cur (it belongs on a later page).
func (k Key) OlderThan(cur Key) bool {
	if k.Date != cur.Date {
		return k.Date < cur.Date
	}
	if k.PeerType != cur.PeerType {
		return k.PeerType > cur.PeerType
	}
	if k.PeerID != cur.PeerID {
		return k.PeerID > cur.PeerID
	}
	return k.MsgID < cur.MsgID
}

// Newer reports that a sorts before b.
func Newer(a, b Key) bool {
	if a.Date != b.Date {
		return a.Date > b.Date
	}
	if a.PeerType != b.PeerType {
		return a.PeerType < b.PeerType
	}
	if a.PeerID != b.PeerID {
		return a.PeerID < b.PeerID
	}
	return a.MsgID > b.MsgID
}
