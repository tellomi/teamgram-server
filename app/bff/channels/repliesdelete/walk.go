// Package repliesdelete walks one user's private dialog with @replies and
// deletes the rows whose forwarded author matches.
package repliesdelete

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/repliesraw"
)

const (
	// Page is one message.getHistoryMessages page. It is also the most ids
	// one delete call carries.
	Page = 100
	// RawOffsetDate asks message.getHistoryMessages for the stored page,
	// including rows the expiry filter would hide, and for database and
	// context errors. Client history reads stay at offset_date >= 0.
	RawOffsetDate = repliesraw.OffsetDate
	// repliesUserID is the @replies service user.
	repliesUserID int64 = 1271266957
)

// PageFunc reads one backward page. offset 0 starts at the newest row.
// The boxes are the stored page, not an expiry-filtered page.
type PageFunc func(ctx context.Context, offset int32) ([]*mtproto.MessageBox, error)

// Delete removes the caller's own box ids. The caller uses PEER_EMPTY and
// revoke false.
type Delete func(ctx context.Context, ids []int32) error

// Author is how a stored @replies row names the person who wrote it.
type Author func(box *mtproto.MessageBox) int64

// BoxLoad reads one of the caller's own live rows. A deleted row is not found.
type BoxLoad func(ctx context.Context, userID int64, msgID int32) (*mtproto.MessageBox, error)

// DeleteHistory loads keepID from the caller's live rows, then walks.
// The author is taken from that row. A missing row is MSG_ID_INVALID.
// keepID is not deleted until Walk finishes.
func DeleteHistory(ctx context.Context, load BoxLoad, read PageFunc, del Delete, self int64, keepID int32) error {
	if load == nil || keepID <= 0 || self == 0 {
		return mtproto.ErrMsgIdInvalid
	}
	box, err := load(ctx, self, keepID)
	if err != nil {
		return err
	}
	if box == nil || box.GetPeerType() != mtproto.PEER_USER || box.GetPeerId() != repliesUserID {
		return mtproto.ErrMsgIdInvalid
	}
	sender := AuthorOf(box)
	if sender == 0 || sender == self || sender == repliesUserID {
		return mtproto.ErrMsgIdInvalid
	}
	return Walk(ctx, read, del, AuthorOf, sender, keepID)
}

// Walk deletes every still-stored row in the caller's @replies dialog whose
// author is sender, except keepID. keepID is deleted only after the walk
// finishes, so a failed call can still load that row and learn the author.
//
// This call covers ids less than or equal to the newest id on the first
// page. A row inserted later with a greater id is not part of this call.
// There is no page cap. The next page is ids strictly below the oldest id
// just read. A full page whose oldest id does not move is an error, not a
// finished scan. Each page is deleted before the next read. Canceling ctx
// stops the walk and returns that error.
func Walk(ctx context.Context, read PageFunc, del Delete, author Author, sender int64, keepID int32) error {
	if read == nil || del == nil || author == nil || sender == 0 || keepID == 0 {
		return mtproto.ErrMsgIdInvalid
	}
	var offset int32
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := read(ctx, offset)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		var oldest int32
		ids := make([]int32, 0, len(rows))
		for _, box := range rows {
			if box == nil {
				continue
			}
			id := box.GetMessageId()
			if id == 0 {
				continue
			}
			if oldest == 0 || id < oldest {
				oldest = id
			}
			if id != keepID && author(box) == sender {
				ids = append(ids, id)
			}
		}
		if err := deleteIDs(ctx, del, ids); err != nil {
			return err
		}
		if len(rows) < Page || oldest == 0 || oldest == offset {
			if len(rows) >= Page && (oldest == 0 || oldest == offset) {
				return mtproto.ErrInternalServerError
			}
			break
		}
		offset = oldest
	}
	return deleteIDs(ctx, del, []int32{keepID})
}

func deleteIDs(ctx context.Context, del Delete, ids []int32) error {
	ids = unique(ids)
	for len(ids) > 0 {
		n := Page
		if n > len(ids) {
			n = len(ids)
		}
		if err := del(ctx, ids[:n]); err != nil {
			return err
		}
		ids = ids[n:]
	}
	return nil
}

// AuthorOf is the forwarded user on an @replies notification.
// The service account itself is not an author. A channel from_id falls
// through to saved_from when that one is a user.
func AuthorOf(box *mtproto.MessageBox) int64 {
	if box == nil || box.GetMessage() == nil || box.GetMessage().GetFwdFrom() == nil {
		if box != nil && box.GetSenderUserId() != 0 && box.GetSenderUserId() != repliesUserID {
			return box.GetSenderUserId()
		}
		return 0
	}
	fwd := box.GetMessage().GetFwdFrom()
	if id := userID(fwd.GetFromId()); id != 0 {
		return id
	}
	if id := userID(fwd.GetSavedFromId()); id != 0 {
		return id
	}
	if box.GetSenderUserId() != 0 && box.GetSenderUserId() != repliesUserID {
		return box.GetSenderUserId()
	}
	return 0
}

func userID(p *mtproto.Peer) int64 {
	if p == nil || p.GetPredicateName() != mtproto.Predicate_peerUser {
		return 0
	}
	return p.GetUserId()
}

func unique(ids []int32) []int32 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int32]bool, len(ids))
	out := make([]int32, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
