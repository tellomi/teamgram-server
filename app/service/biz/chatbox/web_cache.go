package chatbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/mediablob"
	"golang.org/x/sync/singleflight"
	"google.golang.org/protobuf/proto"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

// EnvLinkCacheRedis is the shared page cache. Empty falls back to
// TELLOMI_FLOOD_REDIS, then to memory in this process.
const EnvLinkCacheRedis = "TELLOMI_LINK_CACHE_REDIS"

var webFlight singleflight.Group

type cachedPage struct {
	page  *mtproto.WebPage
	until time.Time
}

type memoryPages struct {
	mu sync.Mutex
	m  map[string]cachedPage
}

func (c *memoryPages) get(key string, now time.Time) (*mtproto.WebPage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ent, ok := c.m[key]
	if !ok || now.After(ent.until) {
		delete(c.m, key)
		return nil, false
	}
	cloned, _ := proto.Clone(ent.page).(*mtproto.WebPage)
	return cloned, cloned != nil
}

func (c *memoryPages) set(key string, page *mtproto.WebPage, ttl time.Duration, now time.Time) {
	cloned, _ := proto.Clone(page).(*mtproto.WebPage)
	if cloned == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]cachedPage{}
	}
	c.m[key] = cachedPage{page: cloned, until: now.Add(ttl)}
}

var localPages = &memoryPages{}

func pageCacheKey(u *url.URL) string {
	if u == nil {
		return ""
	}
	cp := *u
	cp.Scheme = strings.ToLower(cp.Scheme)
	host := strings.ToLower(cp.Hostname())
	if port := cp.Port(); port != "" && !((cp.Scheme == "https" && port == "443") || (cp.Scheme == "http" && port == "80")) {
		host = host + ":" + port
	}
	cp.Host = host
	cp.Fragment = ""
	cp.RawFragment = ""
	sum := sha256.Sum256([]byte(cp.String()))
	return hex.EncodeToString(sum[:])
}

func pageCacheGet(ctx context.Context, key string) (*mtproto.WebPage, bool) {
	if key == "" {
		return nil, false
	}
	if page, ok := redisPageGet(ctx, key); ok {
		return page, true
	}
	return localPages.get(key, time.Now())
}

func pageCacheSet(ctx context.Context, key string, page *mtproto.WebPage, ttl time.Duration) {
	if key == "" || page == nil || ttl <= 0 {
		return
	}
	localPages.set(key, page, ttl, time.Now())
	redisPageSet(ctx, key, page, ttl)
}

var (
	redisOnce sync.Once
	redisCli  *redis.Redis
	blobOnce  sync.Once
	blobStore *mediablob.Store
)

func linkRedis() *redis.Redis {
	redisOnce.Do(func() {
		addr := strings.TrimSpace(os.Getenv(EnvLinkCacheRedis))
		if addr == "" {
			addr = strings.TrimSpace(os.Getenv("TELLOMI_FLOOD_REDIS"))
		}
		if addr == "" {
			return
		}
		r, err := redis.NewRedis(redis.RedisConf{Host: addr, Type: redis.NodeType})
		if err != nil || !r.Ping() {
			return
		}
		redisCli = r
	})
	return redisCli
}

func redisPageGet(ctx context.Context, key string) (*mtproto.WebPage, bool) {
	r := linkRedis()
	if r == nil {
		return nil, false
	}
	raw, err := r.GetCtx(ctx, "webpage:"+key)
	if err != nil || raw == "" {
		return nil, false
	}
	page := &mtproto.WebPage{}
	if err = proto.Unmarshal([]byte(raw), page); err != nil || page.GetPredicateName() == "" {
		return nil, false
	}
	return page, true
}

func redisPageSet(ctx context.Context, key string, page *mtproto.WebPage, ttl time.Duration) {
	r := linkRedis()
	if r == nil {
		return
	}
	raw, err := proto.Marshal(page)
	if err != nil {
		return
	}
	_ = r.SetexCtx(ctx, "webpage:"+key, string(raw), int(ttl/time.Second))
}

func webBlobStore() *mediablob.Store {
	blobOnce.Do(func() {
		dsn := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN"))
		if dsn == "" {
			return
		}
		store, err := mediablob.Open(dsn)
		if err != nil {
			return
		}
		blobStore = store
	})
	return blobStore
}
