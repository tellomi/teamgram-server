package dao

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"google.golang.org/protobuf/proto"
)

// DeliverRepliesNotice writes one @replies inbox row for notice.Recipient
// and, when delivery says so, pushes updateNewMessage to that user only.
// RandomID is the idempotency key of sendMessageToInbox. A second call with
// the same id returns the existing row and does not insert another.
// The comment text is not logged.
func (d *Dao) DeliverRepliesNotice(ctx context.Context, n channel.RepliesNotice, blocked bool, settings []*mtproto.PeerNotifySettings, now int32) error {
	if d == nil || n.Recipient == 0 || n.Message == nil || n.RandomID == 0 {
		return nil
	}
	policy := channel.RepliesDeliveryOf(blocked, now, settings...)
	if !policy.Write {
		return nil
	}
	msg := proto.Clone(n.Message).(*mtproto.Message)
	if policy.Silent {
		msg.Silent = true
	}
	box, _, _, err := d.SendUserMessageToInbox(ctx, channel.RepliesUserID, n.Recipient, 0, n.RandomID, msg)
	if err != nil {
		return err
	}
	if box == nil || box.GetMessageId() == 0 {
		return mtproto.ErrInternalServerError
	}
	if !policy.Push || d.SyncClient == nil || box.GetMessage() == nil {
		return nil
	}
	_, err = d.SyncClient.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{
		UserId: n.Recipient,
		Updates: mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Message_MESSAGE: box.GetMessage(),
			Pts_INT32:       box.GetPts(),
			PtsCount:        box.GetPtsCount(),
		}).To_Update()),
	})
	return err
}
