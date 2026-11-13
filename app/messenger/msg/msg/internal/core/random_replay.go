package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

// replayedOutbox is the updates payload for a random_id this sender already
// committed. ok is false when any id is new, so the caller inserts.
// Official clients resend the same random_id when sendMedia is slow. The
// Redis idempotency cache covers the first minute; this lookup still hits
// after that cache expires and after a request that committed the row but
// did not cache the RPC result.
func (c *MsgCore) replayedOutbox(
	ctx context.Context,
	fromUserID int64,
	randomIDs []int64,
	users func([]int64) []*mtproto.User,
	chats func([]int64) []*mtproto.Chat,
	channels func([]int64) []*mtproto.Chat,
) (*mtproto.Updates, bool) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || len(randomIDs) == 0 {
		return nil, false
	}
	updates := make([]*mtproto.Update, 0, len(randomIDs))
	for _, id := range randomIDs {
		box, ok := c.svcCtx.Dao.FindOutboxByRandomID(ctx, fromUserID, id)
		if !ok {
			return nil, false
		}
		updates = append(updates, outgoingUpdate(box))
	}
	return mtproto.MakeReplyUpdates(users, chats, channels, updates...), true
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
