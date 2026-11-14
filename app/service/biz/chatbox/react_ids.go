package chatbox

import (
	"context"
	"strings"

	"github.com/teamgram/proto/mtproto"
)

// msgIDMap translates private-chat message ids between the two sides.
//
// A private chat stores one row per participant in `messages`. Each row has
// that user's own user_message_box_id (the id the client sees) and a shared
// dialog_message_id. The same message is usually id 250 for one user and
// id 2734 for the other, so a reaction keyed by the reactor's id lands on the
// wrong message (or none) for the other side. Reactions remember the shared
// dialog_message_id and are translated back to the viewer's id on read.
type msgIDMap interface {
	// DialogIDs maps userID's box ids to dialog_message_id.
	DialogIDs(ctx context.Context, userID int64, ids []int32) (map[int32]int64, error)
	// BoxIDs maps dialog_message_id to userID's box ids.
	BoxIDs(ctx context.Context, userID int64, dialogIDs []int64) (map[int64]int32, error)
}

func (s sqlDocs) DialogIDs(ctx context.Context, userID int64, ids []int32) (map[int32]int64, error) {
	out := map[int32]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	args := []any{userID}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT user_message_box_id, dialog_message_id FROM messages
WHERE user_id=? AND deleted=0 AND user_message_box_id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var box int32
		var dlg int64
		if err := rows.Scan(&box, &dlg); err != nil {
			return nil, err
		}
		if dlg != 0 {
			out[box] = dlg
		}
	}
	return out, rows.Err()
}

func (s sqlDocs) BoxIDs(ctx context.Context, userID int64, dialogIDs []int64) (map[int64]int32, error) {
	out := map[int64]int32{}
	if len(dialogIDs) == 0 {
		return out, nil
	}
	args := []any{userID}
	for _, id := range dialogIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT dialog_message_id, user_message_box_id FROM messages
WHERE user_id=? AND deleted=0 AND dialog_message_id IN (`+placeholders(len(dialogIDs))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var dlg int64
		var box int32
		if err := rows.Scan(&dlg, &box); err != nil {
			return nil, err
		}
		out[dlg] = box
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// privatePair is a one-to-one chat with another user, where the two sides
// number the same message differently. Saved Messages is not one of those.
func privatePair(self int64, peer *mtproto.PeerUtil) bool {
	return peer != nil && peer.PeerType == mtproto.PEER_USER && peer.PeerId != 0 && peer.PeerId != self
}

// perUserBox is a chat where each participant has their own
// user_message_box_id for one dialog_message_id. Private chats and basic
// groups both allocate ids that way. Saved Messages and channels do not.
func perUserBox(self int64, peer *mtproto.PeerUtil) bool {
	if peer != nil && peer.PeerType == mtproto.PEER_CHAT && peer.PeerId != 0 {
		return true
	}
	return privatePair(self, peer)
}

// dialogIDLocked is the shared dialog_message_id for self's msgID, or 0 when
// the chat has one id space or the row cannot be read. Caller holds b.mu.
func (b *Book) dialogIDLocked(ctx context.Context, self int64, peer *mtproto.PeerUtil, msgID int32) int64 {
	if b.ids == nil || !perUserBox(self, peer) {
		return 0
	}
	m, err := b.ids.DialogIDs(ctx, self, []int32{msgID})
	if err != nil {
		return 0
	}
	return m[msgID]
}

// sameMessage reports whether item is on the message self calls msgID.
func sameMessage(item reactionItem, msgID int32, dlg int64) bool {
	if dlg != 0 && item.Dialog != 0 {
		return item.Dialog == dlg
	}
	return item.MsgID == msgID
}

// viewLocked returns doc with every item's MsgID in self's numbering. Items on
// messages self no longer has are dropped. Caller holds b.mu.
func (b *Book) viewLocked(ctx context.Context, self int64, peer *mtproto.PeerUtil, doc *reactDoc) *reactDoc {
	if doc == nil || b.ids == nil || !privatePair(self, peer) {
		return doc
	}
	var want []int64
	seen := map[int64]bool{}
	for _, item := range doc.Items {
		if item.Dialog == 0 || item.UserID == self || seen[item.Dialog] {
			continue
		}
		seen[item.Dialog] = true
		want = append(want, item.Dialog)
	}
	if len(want) == 0 {
		return doc
	}
	boxes, err := b.ids.BoxIDs(ctx, self, want)
	if err != nil {
		return doc
	}
	view := *doc
	view.Items = make([]reactionItem, 0, len(doc.Items))
	for _, item := range doc.Items {
		if item.Dialog != 0 && item.UserID != self {
			box, ok := boxes[item.Dialog]
			if !ok {
				continue
			}
			item.MsgID = box
		}
		view.Items = append(view.Items, item)
	}
	return &view
}

// PeerReactionUpdate builds the updateMessageReactions the other side of a
// private chat needs after self reacted to msgID (self's own id). It returns
// the peer's user id and the update in the peer's own numbering, or 0 and nil
// when there is no other side to tell (saved messages, groups, unknown row).
// https://core.telegram.org/constructor/updateMessageReactions
func (b *Book) PeerReactionUpdate(ctx context.Context, self int64, peer *mtproto.PeerUtil, msgID int32) (int64, *mtproto.Update) {
	if b.ids == nil || !privatePair(self, peer) || msgID <= 0 {
		return 0, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	dlg := b.dialogIDLocked(ctx, self, peer, msgID)
	if dlg == 0 {
		return 0, nil
	}
	other := peer.PeerId
	boxes, err := b.ids.BoxIDs(ctx, other, []int64{dlg})
	if err != nil {
		return 0, nil
	}
	theirs, ok := boxes[dlg]
	if !ok || theirs <= 0 {
		return 0, nil
	}
	doc, err := b.loadReact(ctx, self, peer)
	if err != nil {
		return 0, nil
	}
	back := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: self}
	view := b.viewLocked(ctx, other, back, doc)
	return other, reactionUpdate(back, theirs, view, other)
}
