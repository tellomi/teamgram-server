package location

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	// EnvAMapKey is the AMap Web服务 key. Unset: venue search returns an
	// empty list and upload.getWebFile keeps WEBFILE_NOT_AVAILABLE.
	EnvAMapKey = "TELLOMI_AMAP_KEY"
	// EnvAMapSig is the optional 数字签名 secret. It is not a query field.
	EnvAMapSig = "TELLOMI_AMAP_SIG"
	// EnvTencentKey is an optional fallback place (and static-map) source
	// used when AMap is configured but the call fails or returns nothing.
	EnvTencentKey = "TELLOMI_TENCENT_LBS_KEY"
	// EnvQPS is the global upstream cap. Default 3.
	EnvQPS = "TELLOMI_AMAP_QPS"
	// EnvDaily is the shared daily upstream budget. Default 5000.
	EnvDaily = "TELLOMI_AMAP_DAILY"
	// EnvRedis is host:port for the shared cache. Unset uses an in-process cache.
	EnvRedis = "TELLOMI_VENUE_REDIS"
	// EnvAppleTeam is the 10-character Apple Developer Team ID.
	EnvAppleTeam = "TELLOMI_APPLE_MAPS_TEAM_ID"
	// EnvAppleKeyID is the 10-character Maps key id (kid).
	EnvAppleKeyID = "TELLOMI_APPLE_MAPS_KEY_ID"
	// EnvAppleKey is the path of the Maps .p8 private key. All three Apple
	// variables must be set or the Apple provider stays off.
	EnvAppleKey = "TELLOMI_APPLE_MAPS_PRIVATE_KEY_PATH"

	defaultQPS   = 3
	defaultDaily = 5000
	defaultUser  = 20
)

// Options builds a Client. Zero QPS, daily budget, and per-user rate pick
// the defaults above. A nil cache is an in-process map.
type Options struct {
	AMapKey     string
	AMapSig     string
	TencentKey  string
	AMapBase    string
	TencentBase string
	// AppleTeamID, AppleKeyID, and AppleKeyPEM enable the Apple Maps provider.
	// AppleKeyPath is read when AppleKeyPEM is empty.
	AppleTeamID  string
	AppleKeyID   string
	AppleKeyPEM  []byte
	AppleKeyPath string
	AppleBase    string
	HTTP         *http.Client
	Cache        Cache
	QPS          float64
	Daily        int
	UserPerMin   float64
	Now          func() time.Time
}

// Client talks to Apple Maps and AMap. Tencent LBS is the AMap fallback.
type Client struct {
	amapKey     string
	amapSig     string
	tencentKey  string
	amapBase    string
	tencentBase string
	apple       *appleAuth
	http        *http.Client
	cache       Cache
	gate        *gate
	now         func() time.Time

	mu       sync.Mutex
	inflight map[string]*flight
}

type flight struct {
	wg     sync.WaitGroup
	places []place
	next   string
	err    error
}

type clientKey struct{}

// WithClient returns a context Search and StaticMap read instead of the
// process-wide client. Tests use it so they do not touch the environment.
func WithClient(ctx context.Context, c *Client) context.Context {
	return context.WithValue(ctx, clientKey{}, c)
}

// New builds a client. Keys are kept on the struct and never written to logs.
func New(opts Options) *Client {
	if opts.HTTP == nil {
		opts.HTTP = &http.Client{Timeout: upstreamTimeout}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Cache == nil {
		opts.Cache = newMemory(2000, opts.Now)
	}
	if opts.QPS == 0 {
		opts.QPS = defaultQPS
	}
	if opts.Daily == 0 {
		opts.Daily = defaultDaily
	}
	if opts.UserPerMin == 0 {
		opts.UserPerMin = defaultUser
	}
	if opts.AMapBase == "" {
		opts.AMapBase = "https://restapi.amap.com"
	}
	if opts.TencentBase == "" {
		opts.TencentBase = "https://apis.map.qq.com"
	}
	if opts.AppleBase == "" {
		opts.AppleBase = "https://maps-api.apple.com"
	}
	opts.AMapBase = strings.TrimRight(opts.AMapBase, "/")
	opts.TencentBase = strings.TrimRight(opts.TencentBase, "/")
	opts.AppleBase = strings.TrimRight(opts.AppleBase, "/")
	c := &Client{
		amapKey:     opts.AMapKey,
		amapSig:     opts.AMapSig,
		tencentKey:  opts.TencentKey,
		amapBase:    opts.AMapBase,
		tencentBase: opts.TencentBase,
		http:        opts.HTTP,
		cache:       opts.Cache,
		now:         opts.Now,
		inflight:    map[string]*flight{},
	}
	c.apple = newApple(opts, c.now)
	burst := opts.QPS
	if burst < 1 {
		burst = 1
	}
	c.gate = &gate{
		userRate:  opts.UserPerMin / 60,
		userBurst: opts.UserPerMin,
		global:    &bucket{rate: opts.QPS, burst: burst},
		budget:    opts.Daily,
		now:       opts.Now,
		count: func(day string) int {
			return opts.Cache.Incr(context.Background(), "venue:budget:"+day, 48*time.Hour)
		},
		tripRemote: func(until time.Time) {
			opts.Cache.Set(context.Background(), "venue:breaker", []byte(strconv.FormatInt(until.Unix(), 10)), breakerOpen)
		},
		openRemote: func() (time.Time, bool) {
			raw, ok := opts.Cache.Get(context.Background(), "venue:breaker")
			if !ok || len(raw) == 0 {
				return time.Time{}, false
			}
			sec, err := strconv.ParseInt(string(raw), 10, 64)
			if err != nil {
				return time.Time{}, false
			}
			return time.Unix(sec, 0), true
		},
	}
	return c
}

var (
	procOnce sync.Once
	proc     *Client
)

func processClient() *Client {
	procOnce.Do(func() {
		cache := Cache(newMemory(2000, time.Now))
		if addr := strings.TrimSpace(os.Getenv(EnvRedis)); addr != "" {
			if rds := dialRedis(addr); rds != nil {
				cache = &redisCache{r: rds, mem: newMemory(2000, time.Now)}
			}
		}
		proc = New(Options{
			AMapKey:      strings.TrimSpace(os.Getenv(EnvAMapKey)),
			AMapSig:      strings.TrimSpace(os.Getenv(EnvAMapSig)),
			TencentKey:   strings.TrimSpace(os.Getenv(EnvTencentKey)),
			AppleTeamID:  strings.TrimSpace(os.Getenv(EnvAppleTeam)),
			AppleKeyID:   strings.TrimSpace(os.Getenv(EnvAppleKeyID)),
			AppleKeyPath: strings.TrimSpace(os.Getenv(EnvAppleKey)),
			QPS:          envFloat(EnvQPS, defaultQPS),
			Daily:        envInt(EnvDaily, defaultDaily),
			Cache:        cache,
		})
		logx.Infof("venue providers apple=%t amap=%t", proc.apple != nil, proc.amapKey != "")
	})
	return proc
}

func fromContext(ctx context.Context) *Client {
	if ctx != nil {
		if c, ok := ctx.Value(clientKey{}).(*Client); ok && c != nil {
			return c
		}
	}
	return processClient()
}

func envFloat(key string, def float64) float64 {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return def
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envInt(key string, def int) int {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func dialRedis(addr string) *redis.Redis {
	r := redis.New(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if !r.PingCtx(ctx) {
		return nil
	}
	return r
}

type redisCache struct {
	r   *redis.Redis
	mem *memoryCache
}

func (c *redisCache) Get(ctx context.Context, key string) ([]byte, bool) {
	s, err := c.r.GetCtx(ctx, key)
	if err == nil && s != "" {
		return []byte(s), true
	}
	return c.mem.Get(ctx, key)
}

func (c *redisCache) Set(ctx context.Context, key string, val []byte, ttl time.Duration) {
	sec := int(ttl.Seconds())
	if sec < 1 {
		sec = 1
	}
	if err := c.r.SetexCtx(ctx, key, string(val), sec); err != nil {
		c.mem.Set(ctx, key, val, ttl)
	}
}

func (c *redisCache) Incr(ctx context.Context, key string, ttl time.Duration) int {
	n, err := c.r.IncrCtx(ctx, key)
	if err != nil {
		return c.mem.Incr(ctx, key, ttl)
	}
	if n == 1 {
		sec := int(ttl.Seconds())
		if sec < 1 {
			sec = 1
		}
		_ = c.r.ExpireCtx(ctx, key, sec)
	}
	return int(n)
}

func (c *Client) single(key string, fn func() ([]place, string, error)) ([]place, string, error) {
	c.mu.Lock()
	if f, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		f.wg.Wait()
		return f.places, f.next, f.err
	}
	f := &flight{}
	f.wg.Add(1)
	c.inflight[key] = f
	c.mu.Unlock()
	f.places, f.next, f.err = fn()
	f.wg.Done()
	c.mu.Lock()
	delete(c.inflight, key)
	c.mu.Unlock()
	return f.places, f.next, f.err
}
