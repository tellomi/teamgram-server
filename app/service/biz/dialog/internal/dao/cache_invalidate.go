package dao

import (
	"context"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// ExecInvalidate writes through CachedConn.Exec (delete on success) and
// schedules a delayed second delete so an in-flight getDialogs cannot
// SetEx a pre-write DialogExt. Use this for every dialog-row or dialog-
// list cache key, not only setHistoryTTL: pinned, notify, unread, and
// folder_id have the same race.
func (d *Dao) ExecInvalidate(ctx context.Context, exec func(ctx context.Context, conn *sqlx.DB) (int64, int64, error), keys ...string) (int64, int64, error) {
	last, n, _, err := dialog.ExecAndInvalidate(ctx, d.CachedConn, d.InvalidateDelay, exec, keys...)
	return last, n, err
}
