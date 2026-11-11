package dao

import "github.com/teamgram/proto/mtproto"

// inboxMessagePeer is the conversation peer stored on an inbox message row
// and used to look up that dialog's auto-delete timer.
//
// SendUserMessageToInbox historically passes `peer` as the recipient
// (the inbox owner). Private-chat history, dialogs, deletions and the
// TTL backfill treat messages.peer_id as the other user — the same
// value the outbox writes and the dialogs row already uses. Groups and
// channels keep the chat or channel id.
func inboxMessagePeer(fromId int64, peer *mtproto.PeerUtil) *mtproto.PeerUtil {
	if peer != nil && peer.PeerType == mtproto.PEER_USER {
		return &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: fromId}
	}
	return peer
}
