package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

// resumeCommitted finishes a send whose sender rows are already stored.
// done is false when the caller still has to insert. A live row is published
// again: the inbox publish sits outside the insert, so a failed publish or a
// process exit after the commit must not turn the retry into a silent success.
// A deleted row is not published and is not inserted again.
func (c *MsgCore) resumeCommitted(
	ctx context.Context,
	fromUserID int64,
	randomIDs []int64,
	publish func(live []*mtproto.MessageBox) error,
	updates func(all []*mtproto.MessageBox) *mtproto.Updates,
) (bool, *mtproto.Updates, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return false, nil, nil
	}
	kind, live, all, err := c.svcCtx.Dao.ClassifyOutboxReplay(ctx, fromUserID, randomIDs)
	if err != nil {
		return false, nil, err
	}
	return finishReplay(kind, live, all, publish, updates)
}

// finishReplay is the compensation after the sender rows have been classified.
// A failed publish is returned so the idempotency cache is not stored and the
// same random id runs this again.
func finishReplay(
	kind dao.ReplayKind,
	live, all []*mtproto.MessageBox,
	publish func(live []*mtproto.MessageBox) error,
	updates func(all []*mtproto.MessageBox) *mtproto.Updates,
) (bool, *mtproto.Updates, error) {
	switch kind {
	case dao.ReplayMissing:
		return false, nil, nil
	case dao.ReplayDeleted:
		return true, updates(all), nil
	case dao.ReplayLive:
		if err := publish(live); err != nil {
			return true, nil, err
		}
		return true, updates(all), nil
	default:
		return false, nil, nil
	}
}

// alreadyStored is one random id from a multi-message send that died after
// some of the rows committed. skipInsert is set for a live or deleted row so
// the caller does not allocate another id. A deleted row returns a nil box.
func (c *MsgCore) alreadyStored(ctx context.Context, fromUserID, randomID int64) (*mtproto.MessageBox, bool, error) {
	if randomID == 0 || c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, false, nil
	}
	kind, live, _, err := c.svcCtx.Dao.ClassifyOutboxReplay(ctx, fromUserID, []int64{randomID})
	if err != nil {
		return nil, false, err
	}
	switch kind {
	case dao.ReplayLive:
		if len(live) == 0 {
			return nil, false, nil
		}
		return live[0], true, nil
	case dao.ReplayDeleted:
		return nil, true, nil
	default:
		return nil, false, nil
	}
}

func updatesForBoxes(
	boxes []*mtproto.MessageBox,
	users func([]int64) []*mtproto.User,
	chats func([]int64) []*mtproto.Chat,
	channels func([]int64) []*mtproto.Chat,
) *mtproto.Updates {
	list := make([]*mtproto.Update, 0, len(boxes))
	for _, box := range boxes {
		if upd := outgoingUpdate(box); upd != nil {
			list = append(list, upd)
		}
	}
	return mtproto.MakeReplyUpdates(users, chats, channels, list...)
}

func outboxRandomIDs(list []*msg.OutboxMessage) []int64 {
	ids := make([]int64, 0, len(list))
	for _, box := range list {
		if box == nil {
			continue
		}
		ids = append(ids, box.RandomId)
	}
	return ids
}
