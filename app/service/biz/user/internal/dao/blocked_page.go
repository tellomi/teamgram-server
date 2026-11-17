package dao

import (
	"context"
	"errors"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

// BlockedPage is one contacts.getBlocked page plus the full list size.
// offset skips rows. limit is the page size. Order is insertion order.
func (d *Dao) BlockedPage(ctx context.Context, userID int64, offset, limit int32) ([]dataobject.UserPeerBlocksDO, int, error) {
	if d == nil || d.DB == nil || userID == 0 {
		return nil, 0, nil
	}
	var total int
	err := d.DB.QueryRowPartial(ctx, &total, `select count(*) from user_peer_blocks where user_id = ? and deleted = 0`, userID)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 || offset < 0 || int(offset) >= total {
		return nil, total, nil
	}
	var rows []dataobject.UserPeerBlocksDO
	err = d.DB.QueryRowsPartial(ctx, &rows, "select user_id, peer_type, peer_id, `date` from user_peer_blocks where user_id = ? and deleted = 0 order by id asc limit ? offset ?",
		userID, limit, offset)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return nil, total, nil
		}
		return nil, 0, err
	}
	return rows, total, nil
}
