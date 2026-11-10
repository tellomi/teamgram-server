package server

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
	"github.com/teamgram/teamgram-server/app/service/media/internal/config"
	"github.com/teamgram/teamgram-server/app/service/mediapurge"

	"github.com/zeromicro/go-zero/core/logx"
)

// startPurge starts the media purge worker when TELLOMI_MEDIA_PURGE_ENABLED
// is set. Deletes enqueue whether or not this loop is running. A failure
// here does not stop the media service.
func startPurge(c config.Config) context.CancelFunc {
	cfg := mediapurge.FromEnv()
	if !cfg.Enabled {
		logx.Infof("media purge worker disabled (%s is not true)", mediapurge.EnvEnabled)
		return nil
	}
	st, err := mediapurge.Open(c.Mysql.DSN)
	if err != nil {
		logx.Errorf("media purge disabled: %v", err)
		return nil
	}
	if st == nil {
		logx.Infof("media purge disabled (no mysql dsn)")
		return nil
	}
	var del mediapurge.Deleter
	objs, err := objectstore.Open()
	if err != nil {
		logx.Errorf("media purge disabled: %v", err)
		return nil
	}
	if objs != nil {
		del = objs
	} else if !cfg.DryRun {
		logx.Errorf("media purge disabled: object storage (TELLOMI_S3_*) is not configured")
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	logx.Infof("media purge worker: dry_run=%v grace=%s interval=%s batch=%d", cfg.DryRun, cfg.Grace, cfg.Interval, cfg.Batch)
	go st.Loop(ctx, del, cfg)
	return cancel
}
