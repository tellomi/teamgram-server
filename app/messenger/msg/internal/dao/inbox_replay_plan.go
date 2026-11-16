package dao

import (
	"context"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
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
	pts, count, known, err := d.FindNewMessagePts(ctx, row.UserId, row.UserMessageBoxId)
	if err != nil {
		return nil, nil, InboxApply{}, err
	}
	plan := planInboxReplay(true, row.Deleted, known)
	if !plan.Push && !plan.MoveTop {
		return nil, nil, InboxApply{}, nil
	}
	box := makeMessageBoxByDO(row)
	if known {
		box.Pts, box.PtsCount = pts, count
	} else if d != nil {
		box.Pts = d.IDGenClient2.NextPtsId(ctx, row.UserId)
		box.PtsCount = 1
	}
	if plan.MoveTop {
		d.touchReplayDialog(ctx, row)
	}
	return box, nil, InboxApply{PersistPts: plan.PersistPts, Push: plan.Push}, nil
}

// touchReplayDialog moves the recipient dialog forward without adding unread.
// The first insert already added it. A retry must not add it again, and a
// smaller id must not pass GREATEST.
func (d *Dao) touchReplayDialog(ctx context.Context, row *dataobject.MessagesDO) {
	if d == nil || d.DialogsDAO == nil || row == nil || row.UserMessageBoxId == 0 {
		return
	}
	dialogDO := &dataobject.DialogsDO{
		UserId:           row.UserId,
		PeerType:         row.PeerType,
		PeerId:           row.PeerId,
		PeerDialogId:     mtproto.MakePeerDialogId(row.PeerType, row.PeerId),
		TopMessage:       row.UserMessageBoxId,
		UnreadCount:      0,
		DraftMessageData: "null",
		Date2:            row.Date2,
	}
	_, _, _ = d.ExecInvalidate(ctx, func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
		_, _, err := d.DialogsDAO.InsertOrUpdateForward(ctx, dialogDO)
		return 0, 0, err
	}, dialog.GetDialogCacheKey(dialogDO.UserId, dialogDO.PeerDialogId))
}
