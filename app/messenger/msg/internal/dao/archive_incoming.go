package dao

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/archive"
)

// ArchiveIncoming applies official archive rules for one incoming message and
// returns the updates to deliver with updateNewMessage. The folder change is
// stored and given the next pts after the message pts.
func (d *Dao) ArchiveIncoming(ctx context.Context, in archive.Incoming) []*mtproto.Update {
	if d == nil || d.DialogClient == nil || d.UserClient == nil {
		return nil
	}
	gen := d.IDGenClient2
	return archive.Apply(ctx, archive.Clients{
		Dialog: d.DialogClient,
		User:   d.UserClient,
	}, archive.SeqWriter{
		Next: &gen,
		Save: archive.SavePtsSQL(d.DB),
	}, in)
}
