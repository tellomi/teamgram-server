package dao

import (
	"context"

	"github.com/teamgram/marmota/pkg/hack"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// insertSenderSearchRow writes the sender's messages row before send returns.
//
// The inbox consumer inserts that same row only after Kafka has accepted
// the record. messages.getSearchCounters reads the row. A photo sent a
// moment earlier is already committed, so its message_filter_type count is
// 1, while the voice note just sent is still 0. Writing the row here makes
// the filter type visible to that count on the same connection commit.
//
// The consumer inserts again. InsertOrReturnId treats the same box id and
// random id as a replay and does not overwrite the row. Dialog top_message
// stays on the consumer, which applies sends in partition order.
func (d *Dao) insertSenderSearchRow(ctx context.Context, peer *mtproto.PeerUtil, outMsgBox *mtproto.MessageBox) error {
	if d == nil || d.DB == nil || d.MessagesDAO == nil || peer == nil || outMsgBox == nil || outMsgBox.Message == nil {
		return mtproto.ErrInternalServerError
	}
	message := outMsgBox.Message
	message.SanitizeEntities()
	d.stampHistoryTTL(ctx, outMsgBox.UserId, peer, message)
	mData, err := jsonx.Marshal(message)
	if err != nil {
		return err
	}
	saved := &mtproto.PeerUtil{PeerType: mtproto.PEER_EMPTY, PeerId: 0}
	if message.GetSavedPeerId() != nil {
		saved = mtproto.FromPeer(message.GetSavedPeerId())
	}
	tR := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		_, _, err := d.MessagesDAO.InsertOrReturnIdTx(tx, &dataobject.MessagesDO{
			UserId:            outMsgBox.UserId,
			UserMessageBoxId:  outMsgBox.MessageId,
			DialogId1:         outMsgBox.DialogId1,
			DialogId2:         outMsgBox.DialogId2,
			DialogMessageId:   outMsgBox.DialogMessageId,
			SenderUserId:      outMsgBox.UserId,
			PeerType:          peer.PeerType,
			PeerId:            peer.PeerId,
			RandomId:          outMsgBox.RandomId,
			MessageFilterType: outMsgBox.MessageFilterType,
			MessageData:       hack.String(mData),
			Message:           message.GetMessage(),
			Mentioned:         false,
			MediaUnread:       message.GetMediaUnread(),
			Date2:             int64(message.GetDate()),
			TtlPeriod:         historyPeriod(message),
			SavedPeerType:     saved.PeerType,
			SavedPeerId:       saved.PeerId,
			Deleted:           false,
		})
		if err != nil {
			result.Err = err
		}
	})
	return tR.Err
}
