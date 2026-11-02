package server

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
	"github.com/teamgram/teamgram-server/app/service/media/internal/config"
	"github.com/teamgram/teamgram-server/app/service/media/retention"

	"github.com/zeromicro/go-zero/core/logx"
)

// startRetention starts the daily media sweeper when
// TELLOMI_MEDIA_RETENTION_ENABLED is set. Failures only disable the sweeper;
// they never stop the media service.
func startRetention(c config.Config) context.CancelFunc {
	cfg, err := retention.FromEnv()
	if err != nil {
		logx.Errorf("media retention disabled: %v", err)
		return nil
	}
	if !cfg.Enabled {
		logx.Infof("media retention disabled (%s is not true)", retention.EnvEnabled)
		return nil
	}
	sw, err := NewSweeper(c, cfg)
	if err != nil {
		logx.Errorf("media retention disabled: %v", err)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	go sw.Schedule(ctx)
	return cancel
}

// NewSweeper builds a sweeper from the media config and the TELLOMI_S3_*
// object storage settings.
func NewSweeper(c config.Config, cfg retention.Config) (*retention.Sweeper, error) {
	store, err := retention.OpenSQL(c.Mysql.DSN)
	if err != nil {
		return nil, err
	}
	sw := &retention.Sweeper{Cfg: cfg, Store: store, Logf: logx.Infof}
	objs, err := objectstore.Open()
	if err != nil {
		return nil, err
	}
	if objs != nil {
		sw.Objects = objs
	} else if !cfg.DryRun {
		return nil, errNoObjectStore
	}
	return sw, nil
}

type retentionError string

func (e retentionError) Error() string { return string(e) }

const errNoObjectStore = retentionError("object storage (TELLOMI_S3_*) is not configured")
