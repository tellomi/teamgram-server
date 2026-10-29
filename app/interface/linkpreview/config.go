package linkpreview

import (
	"crypto/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

// HeaderToken is the shared secret the Cloudflare Worker sends.
const HeaderToken = "X-Tellomi-Linkpreview-Token"

// Config is the link preview listener. Environment variables override a yaml file.
type Config struct {
	Listen               string    `json:",optional"`
	MysqlDSN             string    `json:",optional"`
	LinkBaseURL          string    `json:",optional"`
	PublicURL            string    `json:",optional"`
	PublicMediaBaseURL   string    `json:",optional"`
	Token                string    `json:",optional"`
	MediaKey             string    `json:",optional"`
	RatePerMin           int       `json:",optional"`
	RateBurst            int       `json:",optional"`
	CacheSeconds         int       `json:",optional"`
	NotFoundCacheSeconds int       `json:",optional"`
	MediaTTLSeconds      int       `json:",optional"`
	TrustProxy           bool      `json:",optional"`
	MinioEndpoint        string    `json:",optional"`
	MinioAccessKey       string    `json:",optional"`
	MinioSecretKey       string    `json:",optional"`
	MinioSSL             bool      `json:",optional"`
	MinioBucketPhotos    string    `json:",optional"`
	MinioBucketDocuments string    `json:",optional"`
	MinioBucketVideos    string    `json:",optional"`
	Log                  LogConfig `json:",optional"`

	ephemeralKey []byte
}

// LogConfig is the go-zero file log used when Mode is "file".
type LogConfig struct {
	ServiceName string `json:",optional"`
	Mode        string `json:",optional"`
	Path        string `json:",optional"`
	Level       string `json:",optional"`
	KeepDays    int    `json:",optional"`
	Rotation    string `json:",optional"`
}

// Load reads an optional yaml file and applies TELLOMI_* environment variables.
func Load(path string) Config {
	cfg := defaults()
	if path != "" {
		conf.MustLoad(path, &cfg)
		cfg = cfg.withDefaults()
	}
	cfg.applyEnv()
	if len(cfg.ephemeralKey) == 0 {
		cfg.ephemeralKey = make([]byte, 32)
		_, _ = rand.Read(cfg.ephemeralKey)
	}
	return cfg
}

func defaults() Config {
	return Config{
		Listen:               ":8801",
		LinkBaseURL:          "https://tell.cc",
		RatePerMin:           120,
		RateBurst:            40,
		CacheSeconds:         120,
		NotFoundCacheSeconds: 30,
		MediaTTLSeconds:      86400,
		MinioBucketPhotos:    "photos",
		MinioBucketDocuments: "documents",
		MinioBucketVideos:    "videos",
	}.withKey()
}

func (c Config) withKey() Config {
	if len(c.ephemeralKey) == 0 {
		c.ephemeralKey = make([]byte, 32)
		_, _ = rand.Read(c.ephemeralKey)
	}
	return c
}

func (c Config) withDefaults() Config {
	d := defaults()
	if c.Listen == "" {
		c.Listen = d.Listen
	}
	if c.LinkBaseURL == "" {
		c.LinkBaseURL = d.LinkBaseURL
	}
	if c.RatePerMin <= 0 {
		c.RatePerMin = d.RatePerMin
	}
	if c.RateBurst <= 0 {
		c.RateBurst = d.RateBurst
	}
	if c.CacheSeconds <= 0 {
		c.CacheSeconds = d.CacheSeconds
	}
	if c.NotFoundCacheSeconds <= 0 {
		c.NotFoundCacheSeconds = d.NotFoundCacheSeconds
	}
	if c.MediaTTLSeconds <= 0 {
		c.MediaTTLSeconds = d.MediaTTLSeconds
	}
	if c.MinioBucketPhotos == "" {
		c.MinioBucketPhotos = d.MinioBucketPhotos
	}
	if c.MinioBucketDocuments == "" {
		c.MinioBucketDocuments = d.MinioBucketDocuments
	}
	if c.MinioBucketVideos == "" {
		c.MinioBucketVideos = d.MinioBucketVideos
	}
	if len(c.ephemeralKey) == 0 {
		c.ephemeralKey = d.ephemeralKey
	}
	return c
}

func (c *Config) applyEnv() {
	set := func(key string, dst *string) {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			*dst = v
		}
	}
	if v, ok := os.LookupEnv("TELLOMI_LINKPREVIEW_ADDR"); ok && v != "" {
		c.Listen = v
	}
	set("TELLOMI_LINKPREVIEW_MYSQL_DSN", &c.MysqlDSN)
	if c.MysqlDSN == "" {
		set("TEAMGRAM_MYSQL_DSN", &c.MysqlDSN)
	}
	set("TELLOMI_LINK_BASE_URL", &c.LinkBaseURL)
	set("TELLOMI_LINKPREVIEW_PUBLIC_URL", &c.PublicURL)
	set("TELLOMI_S3_PUBLIC_MEDIA_BASE_URL", &c.PublicMediaBaseURL)
	set("TELLOMI_LINKPREVIEW_TOKEN", &c.Token)
	set("TELLOMI_LINKPREVIEW_MEDIA_KEY", &c.MediaKey)
	set("TELLOMI_MINIO_ENDPOINT", &c.MinioEndpoint)
	set("TELLOMI_MINIO_ACCESS_KEY", &c.MinioAccessKey)
	set("TELLOMI_MINIO_SECRET_KEY", &c.MinioSecretKey)
	set("TELLOMI_MINIO_BUCKET_PHOTOS", &c.MinioBucketPhotos)
	set("TELLOMI_MINIO_BUCKET_DOCUMENTS", &c.MinioBucketDocuments)
	set("TELLOMI_MINIO_BUCKET_VIDEOS", &c.MinioBucketVideos)
	if v, ok := os.LookupEnv("TELLOMI_LINKPREVIEW_RATE_PER_MIN"); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			panic("TELLOMI_LINKPREVIEW_RATE_PER_MIN must be a positive integer")
		}
		c.RatePerMin = n
	}
	if v, ok := os.LookupEnv("TELLOMI_LINKPREVIEW_RATE_BURST"); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			panic("TELLOMI_LINKPREVIEW_RATE_BURST must be a positive integer")
		}
		c.RateBurst = n
	}
	if v, ok := os.LookupEnv("TELLOMI_LINKPREVIEW_CACHE_SECONDS"); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			panic("TELLOMI_LINKPREVIEW_CACHE_SECONDS must be a non-negative integer")
		}
		c.CacheSeconds = n
	}
	if v, ok := os.LookupEnv("TELLOMI_LINKPREVIEW_TRUST_PROXY"); ok && v != "" {
		c.TrustProxy = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	if v, ok := os.LookupEnv("TELLOMI_MINIO_USE_SSL"); ok && v != "" {
		c.MinioSSL = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	c.LinkBaseURL = strings.TrimRight(c.LinkBaseURL, "/")
	c.PublicURL = strings.TrimRight(c.PublicURL, "/")
	c.PublicMediaBaseURL = strings.TrimRight(c.PublicMediaBaseURL, "/")
}

// SetupLog writes linkpreview logs to a rotated file when configured.
// Docker mounts ./data/logs at /app/logs. An empty Mode keeps stderr.
func (c Config) SetupLog() {
	if c.Log.Mode != "file" {
		return
	}
	path := c.Log.Path
	if path == "" {
		path = "/app/logs/linkpreview"
	}
	level := c.Log.Level
	if level == "" {
		level = "info"
	}
	keep := c.Log.KeepDays
	if keep <= 0 {
		keep = 7
	}
	rotation := c.Log.Rotation
	if rotation == "" {
		rotation = "daily"
	}
	name := c.Log.ServiceName
	if name == "" {
		name = "interface.linkpreview"
	}
	logx.MustSetup(logx.LogConf{
		ServiceName: name,
		Mode:        "file",
		Encoding:    "json",
		Path:        path,
		Level:       level,
		KeepDays:    keep,
		Rotation:    rotation,
	})
}

func (c Config) mediaTTL() time.Duration {
	if c.MediaTTLSeconds <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(c.MediaTTLSeconds) * time.Second
}

func (c Config) mediaKey() []byte {
	if c.MediaKey != "" {
		return []byte(c.MediaKey)
	}
	if c.Token != "" {
		return []byte("linkpreview-media\x00" + c.Token)
	}
	return c.ephemeralKey
}
