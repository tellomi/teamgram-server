package core

import "github.com/teamgram/proto/mtproto"

// savedDraftPeer stores a Saved Messages draft on the same dialog row the
// inbox uses: peer_type user and peer_id = self. InputPeerSelf arrives as
// PEER_SELF, and an UPDATE against that type matches zero rows.
func savedDraftPeer(userID int64, peerType int32, peerID int64) (int32, int64) {
	if peerType == mtproto.PEER_SELF || (peerType == mtproto.PEER_USER && (peerID == 0 || peerID == userID)) {
		return mtproto.PEER_USER, userID
	}
	return peerType, peerID
}
