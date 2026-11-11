package dao

import (
	"context"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// ExecInvalidate is the dialog-row write helper: CachedConn.Exec plus a
// delayed second DelCache, so an in-flight getDialogs cannot SetEx a
// pre-write object (unread, top_message, ttl_period, and the rest).
func (d *Dao) ExecInvalidate(ctx context.Context, exec func(ctx context.Context, conn *sqlx.DB) (int64, int64, error), keys ...string) (int64, int64, error) {
	last, n, _, err := dialog.ExecAndInvalidate(ctx, d.CachedConn, 0, exec, keys...)
	return last, n, err
}
