package core

import "github.com/teamgram/proto/mtproto"

// draftStorePeer matches the saved-messages dialog row (user + self).
func draftStorePeer(self int64, peer *mtproto.PeerUtil) *mtproto.PeerUtil {
	if peer == nil {
		return nil
	}
	if peer.PeerType == mtproto.PEER_SELF || (peer.PeerType == mtproto.PEER_USER && (peer.PeerId == 0 || peer.PeerId == self)) {
		peer.PeerType = mtproto.PEER_USER
		peer.PeerId = self
	}
	return peer
}
