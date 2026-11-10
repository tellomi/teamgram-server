package dao

import (
	"os"
	"strings"

	"github.com/teamgram/teamgram-server/app/interface/session/internal/config"
	"github.com/teamgram/teamgram-server/pkg/flood"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// openFlood connects the Redis from TELLOMI_FLOOD_REDIS or session.yaml Cache.
// A missing or unreachable Redis leaves limits off. Chat must keep working
// when the cache is down; the next process start tries again.
func openFlood(c config.Config) *flood.Limiter {
	cfg := flood.ConfigFromEnv()
	if cfg.Mode == flood.ModeOff {
		logx.Info("flood: off")
		return nil
	}
	conf := redis.RedisConf{Type: redis.NodeType}
	if addr := strings.TrimSpace(os.Getenv("TELLOMI_FLOOD_REDIS")); addr != "" {
		conf.Host = addr
	} else if len(c.Cache) > 0 && strings.TrimSpace(c.Cache[0].Host) != "" {
		conf = c.Cache[0].RedisConf
		if conf.Type == "" {
			conf.Type = redis.NodeType
		}
	}
	if conf.Host == "" {
		logx.Info("flood: no redis configured, rate limits are off")
		return nil
	}
	r, err := redis.NewRedis(conf)
	if err != nil || !r.Ping() {
		logx.Errorf("flood: redis %s unavailable, rate limits are off: %v", conf.Host, err)
		return nil
	}
	logx.Infof("flood: %s via redis %s", cfg.Mode, conf.Host)
	return flood.New(flood.NewRedisStore(r), cfg)
}
