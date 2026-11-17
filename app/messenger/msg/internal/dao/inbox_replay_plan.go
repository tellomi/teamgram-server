package dao

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
)

// InboxApply tells the inbox handler whether to store a new pts row and
// whether to push updateNewMessage. A deleted replay does neither.
type InboxApply struct {
	PersistPts bool
	Push       bool
}

// InboxReplayPlan is the recipient apply after a random id is looked up.
// The first insert adds unread and allocates one pts. A later apply of a
// live row may move top_message forward and may push the original pts to
// another device. It does not add unread and it does not allocate a second
// pts when that pts row is already stored. A deleted row is left alone.
type InboxReplayPlan struct {
	Insert     bool
	Push       bool
	MoveTop    bool
	AddUnread  bool
	PersistPts bool
}

func planInboxReplay(found, deleted, ptsKnown bool) InboxReplayPlan {
	if !found {
		return InboxReplayPlan{Insert: true, Push: true, MoveTop: true, AddUnread: true, PersistPts: true}
	}
	if deleted {
		return InboxReplayPlan{}
	}
	// Unread is committed with the message. A replay must not add it again.
	return InboxReplayPlan{
		Insert:     false,
		Push:       true,
		MoveTop:    true,
		AddUnread:  false,
		PersistPts: !ptsKnown,
	}
}

func (d *Dao) replayExistingInbox(ctx context.Context, row *dataobject.MessagesDO) (*mtproto.MessageBox, *AnsweredCallRead, InboxApply, error) {
	if row == nil {
		return nil, nil, InboxApply{}, nil
	}
	if row.Deleted {
		return nil, nil, InboxApply{}, nil
	}
	pts, count, known, err := d.FindNewMessagePts(ctx, row.UserId, row.UserMessageBoxId)
	if err != nil {
		return nil, nil, InboxApply{}, err
	}
	box := makeMessageBoxByDO(row)
	if known {
		box.Pts, box.PtsCount = pts, count
	}
	return box, nil, InboxApply{PersistPts: !known, Push: true}, nil
}
