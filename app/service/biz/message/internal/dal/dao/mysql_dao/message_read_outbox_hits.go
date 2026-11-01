package mysql_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/pkg/readtime"
	"github.com/zeromicro/go-zero/core/logx"
)

// OutboxReadHit is one reader whose checkpoint already covers the message.
// ReadDate is a Unix second, or 0 when neither the column nor updated_at has one.
type OutboxReadHit struct {
	ReadUserId int64 `db:"read_user_id"`
	ReadDate   int64 `db:"read_date"`
}

// SelectOutboxReadHits returns readers of ownerID's outbox in peerDialogID.
// oneReader restricts the result to readerID (private chats).
func (dao *MessageReadOutboxDAO) SelectOutboxReadHits(ctx context.Context, ownerID, peerDialogID, readerID int64, msgID int32, oneReader bool) (hits []OutboxReadHit, err error) {
	args := []interface{}{ownerID, peerDialogID, msgID}
	if oneReader {
		args = append(args, readerID)
	}
	err = dao.db.QueryRowsPartial(ctx, &hits, readtime.SelectHitsSQL(oneReader, true), args...)
	if err != nil {
		// Older tables have the date column and not updated_at. Fall back
		// rather than failing the RPC.
		hits = nil
		err = dao.db.QueryRowsPartial(ctx, &hits, readtime.SelectHitsSQL(oneReader, false), args...)
	}
	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in SelectOutboxReadHits(_), error: %v", err)
		return nil, err
	}
	return hits, nil
}
