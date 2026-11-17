package dao

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// SenderChatDialog is the creator's dialog row for a basic-group send.
// messages.createChat returns before the inbox consumer runs, and
// messages.toggleDialogPin reads this row. It has to exist first.
// Only top_message and date2 are set. insertOrUpdateDialog writes
// unread_count as an absolute value, so leaving it unset keeps an
// existing unread count. A new row still starts at 0 via InsertIgnore.
func SenderChatDialog(fromID, chatID int64, box *mtproto.MessageBox) *dialog.TLDialogInsertOrUpdateDialog {
	var (
		top  int32
		date int64
	)
	if box != nil {
		top = box.MessageId
		if box.Message != nil {
			date = int64(box.Message.Date)
		}
	}
	return &dialog.TLDialogInsertOrUpdateDialog{
		UserId:     fromID,
		PeerType:   mtproto.PEER_CHAT,
		PeerId:     chatID,
		TopMessage: &wrapperspb.Int32Value{Value: top},
		Date2:      &wrapperspb.Int64Value{Value: date},
	}
}

// NewestOutbox is the box whose message id should be the dialog top.
func NewestOutbox(boxes []*mtproto.MessageBox) *mtproto.MessageBox {
	var top *mtproto.MessageBox
	for _, box := range boxes {
		if box == nil {
			continue
		}
		if top == nil || box.MessageId >= top.MessageId {
			top = box
		}
	}
	return top
}

// WriteSenderChatDialog inserts the sender's basic-group dialog before the
// send RPC returns. A later inbox apply moves top_message forward and does
// not clear pinned.
func (d *Dao) WriteSenderChatDialog(ctx context.Context, fromID, chatID int64, box *mtproto.MessageBox) error {
	if d == nil || d.DialogClient == nil {
		return mtproto.ErrInternalServerError
	}
	_, err := d.DialogClient.DialogInsertOrUpdateDialog(ctx, SenderChatDialog(fromID, chatID, box))
	return err
}

// WriteSenderChatDialogs uses the newest box in a multi-message send.
func (d *Dao) WriteSenderChatDialogs(ctx context.Context, fromID, chatID int64, boxes []*mtproto.MessageBox) error {
	return d.WriteSenderChatDialog(ctx, fromID, chatID, NewestOutbox(boxes))
}
