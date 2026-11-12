package core

import "github.com/teamgram/proto/mtproto"

// answeredCallReadUpdate is updateReadHistoryInbox for a call the callee
// already took. still_unread_count is 0 because the read cursor moved to
// this message, which is the newest one in the dialog.
func answeredCallReadUpdate(fromID int64, messageID, pts int32) *mtproto.Update {
	return mtproto.MakeTLUpdateReadHistoryInbox(&mtproto.Update{
		Peer_PEER:        mtproto.MakePeerUser(fromID),
		MaxId:            messageID,
		StillUnreadCount: 0,
		Pts_INT32:        pts,
		PtsCount:         1,
	}).To_Update()
}
