package dao

import "context"

// savedPeerDialogSQL inserts a Saved Messages peer dialog. A second apply of
// an older message must not rewind top_message or clear deleted. deleted is
// cleared only when this id is newer than the stored top, which is how a
// later live message brings the peer back. The comparison reads the stored
// top before the assignment below replaces it.
const savedPeerDialogSQL = "" +
	"insert into saved_dialogs(user_id, peer_type, peer_id, pinned, top_message) " +
	"values (?, ?, ?, 0, ?) " +
	"on duplicate key update " +
	"deleted = if(top_message < values(top_message), 0, deleted), " +
	"top_message = greatest(top_message, values(top_message))"

// TouchSavedPeerDialog records one saved-message peer. Call it only for a
// message row that is still live. A deleted row must not be passed in: this
// statement would otherwise treat that id as a newer top.
func (d *Dao) TouchSavedPeerDialog(ctx context.Context, userID int64, peerType int32, peerID int64, top int32) error {
	if d == nil || d.DB == nil || userID == 0 || top <= 0 {
		return nil
	}
	_, err := d.DB.Exec(ctx, savedPeerDialogSQL, userID, peerType, peerID, top)
	return err
}
