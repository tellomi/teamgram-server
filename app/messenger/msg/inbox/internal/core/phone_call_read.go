package core

import "github.com/teamgram/proto/mtproto"

// answeredCallReadUpdate is updateReadHistoryInbox for a call the callee
// already took, and only when the read cursor could move onto that message
// without marking an earlier incoming message read. stillUnread is the
// dialog's remaining unread count after the move.
func answeredCallReadUpdate(fromID int64, messageID, stillUnread, pts int32) *mtproto.Update {
	return mtproto.MakeTLUpdateReadHistoryInbox(&mtproto.Update{
		Peer_PEER:        mtproto.MakePeerUser(fromID),
		MaxId:            messageID,
		StillUnreadCount: stillUnread,
		Pts_INT32:        pts,
		PtsCount:         1,
	}).To_Update()
}
