package linkpreview

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Handler is the public HTTP API.
type Handler struct {
	svc   *Service
	cfg   Config
	lim   *limiter
	cache *respCache
	minio *minio.Client
}

func NewHandler(cfg Config, cat Catalog) *Handler {
	cfg = cfg.withDefaults()
	h := &Handler{
		svc:   New(cfg, cat),
		cfg:   cfg,
		lim:   newLimiter(cfg.RatePerMin, cfg.RateBurst),
		cache: newRespCache(),
	}
	if cfg.MinioEndpoint != "" && cfg.MinioAccessKey != "" {
		cl, err := minio.New(cfg.MinioEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
			Secure: cfg.MinioSSL,
		})
		if err != nil {
			log.Printf("linkpreview: minio: %v", err)
		} else {
			h.minio = cl
		}
	}
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/linkpreview/v1/health" && r.Method == http.MethodGet:
		h.writeJSON(w, r, http.StatusOK, &Card{OK: true, SchemaVersion: SchemaVersion}, "no-store")
	case r.URL.Path == "/linkpreview/v1/resolve" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		h.resolve(w, r)
	case r.URL.Path == "/linkpreview/v1/media" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		h.media(w, r)
	default:
		h.writeErr(w, r, http.StatusNotFound, ErrNotFound, "")
	}
}

func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		h.writeErr(w, r, http.StatusUnauthorized, ErrUnauthorized, "")
		return
	}
	if !h.lim.allow(clientIP(r, h.cfg.TrustProxy), time.Now()) {
		w.Header().Set("Retry-After", "1")
		h.writeErr(w, r, http.StatusTooManyRequests, ErrRateLimited, "")
		return
	}
	raw := r.URL.Query().Get("path")
	if len(raw) > 2048 {
		h.writeErr(w, r, http.StatusBadRequest, ErrInvalidPath, "")
		return
	}
	key := r.URL.RawQuery
	if status, body, etag, cc, ok := h.cache.get(key, time.Now()); ok {
		h.writeRaw(w, r, status, body, etag, cc)
		return
	}
	extra := r.URL.Query()
	extra.Del("path")
	card, err := h.svc.Resolve(r.Context(), raw, extra)
	if err != nil {
		code := errorCode(err)
		status := statusFor(code)
		if code == ErrInternal {
			log.Printf("linkpreview: %v", err)
		}
		body := h.writeErr(w, r, status, code, safePath(raw))
		if code == ErrNotFound {
			h.cache.put(key, status, body, cacheControl(h.cfg, false), time.Now().Add(time.Duration(h.cfg.NotFoundCacheSeconds)*time.Second))
		}
		return
	}
	cc := cacheControl(h.cfg, true)
	body := h.writeJSON(w, r, http.StatusOK, card, cc)
	h.cache.put(key, http.StatusOK, body, cc, time.Now().Add(time.Duration(h.cfg.CacheSeconds)*time.Second))
}

func (h *Handler) media(w http.ResponseWriter, r *http.Request) {
	if !h.lim.allow(clientIP(r, h.cfg.TrustProxy), time.Now()) {
		w.Header().Set("Retry-After", "1")
		h.writeErr(w, r, http.StatusTooManyRequests, ErrRateLimited, "")
		return
	}
	q := r.URL.Query()
	path := sanitizeMediaPath(q.Get("p"))
	exp, err := strconv.ParseInt(q.Get("exp"), 10, 64)
	if path == "" || err != nil || !verifyMedia(h.cfg.mediaKey(), path, exp, q.Get("sig"), time.Now()) {
		h.writeErr(w, r, http.StatusNotFound, ErrNotFound, "")
		return
	}
	if h.cfg.PublicMediaBaseURL != "" {
		http.Redirect(w, r, h.cfg.PublicMediaBaseURL+"/"+escapeMediaPath(path), http.StatusFound)
		return
	}
	if h.minio == nil {
		h.writeErr(w, r, http.StatusNotFound, ErrMediaUnavailable, "")
		return
	}
	obj, stat, err := h.openObject(r.Context(), path)
	if err != nil {
		h.writeErr(w, r, http.StatusNotFound, ErrNotFound, "")
		return
	}
	defer obj.Close()
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if stat.ContentType != "" {
		w.Header().Set("Content-Type", stat.ContentType)
	}
	http.ServeContent(w, r, path, stat.LastModified, obj)
}

func (h *Handler) openObject(ctx context.Context, path string) (*minio.Object, minio.ObjectInfo, error) {
	buckets := []string{h.cfg.MinioBucketPhotos, h.cfg.MinioBucketDocuments, h.cfg.MinioBucketVideos}
	var last error
	for _, b := range buckets {
		if b == "" {
			continue
		}
		obj, err := h.minio.GetObject(ctx, b, path, minio.GetObjectOptions{})
		if err != nil {
			last = err
			continue
		}
		stat, err := obj.Stat()
		if err != nil {
			_ = obj.Close()
			last = err
			continue
		}
		return obj, stat, nil
	}
	if last == nil {
		last = errNotFound
	}
	return nil, minio.ObjectInfo{}, last
}

func (h *Handler) authorized(r *http.Request) bool {
	if h.cfg.Token == "" {
		return true
	}
	got := r.Header.Get(HeaderToken)
	if got == "" {
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if strings.HasPrefix(auth, prefix) {
			got = strings.TrimPrefix(auth, prefix)
		}
	}
	if len(got) != len(h.cfg.Token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.cfg.Token)) == 1
}

func cacheControl(cfg Config, hit bool) string {
	scope := "public"
	if cfg.Token != "" {
		scope = "private"
	}
	seconds := cfg.CacheSeconds
	if !hit {
		seconds = cfg.NotFoundCacheSeconds
	}
	if seconds < 0 {
		seconds = 0
	}
	return scope + ", max-age=" + strconv.Itoa(seconds)
}

func statusFor(code string) int {
	switch code {
	case ErrInvalidPath:
		return http.StatusBadRequest
	case ErrUnauthorized:
		return http.StatusUnauthorized
	case ErrRateLimited:
		return http.StatusTooManyRequests
	case ErrInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusNotFound
	}
}

func safePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		raw = raw[:i]
	}
	if len(raw) > 256 {
		raw = raw[:256]
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		return u.Path
	}
	return raw
}

func clientIP(r *http.Request, trust bool) string {
	if trust {
		if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
			return ip
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if first != "" {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *Handler) writeErr(w http.ResponseWriter, r *http.Request, status int, code, path string) []byte {
	cc := "no-store"
	if code == ErrNotFound {
		cc = cacheControl(h.cfg, false)
	}
	return h.writeJSON(w, r, status, errorCard(code, path), cc)
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, card *Card, cc string) []byte {
	body, err := json.Marshal(card)
	if err != nil {
		body = []byte(`{"ok":false,"schema_version":1,"error":"internal"}`)
		status = http.StatusInternalServerError
		cc = "no-store"
	}
	h.writeRaw(w, r, status, body, etagOf(body), cc)
	return body
}

func (h *Handler) writeRaw(w http.ResponseWriter, r *http.Request, status int, body []byte, etag, cc string) {
	if cc == "" {
		cc = "no-store"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", cc)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Vary", HeaderToken)
	if etag != "" {
		w.Header().Set("ETag", etag)
		if status == http.StatusOK && etagMatches(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func etagOf(body []byte) string {
	sum := sha256.Sum256(body)
	hexed := hex.EncodeToString(sum[:])
	return `"` + hexed[:16] + `"`
}

func etagMatches(header, etag string) bool {
	if header == "" || etag == "" {
		return false
	}
	return strings.Contains(header, etag)
}

type respCache struct {
	mu sync.Mutex
	m  map[string]cacheEntry
}

type cacheEntry struct {
	status int
	body   []byte
	etag   string
	cc     string
	exp    time.Time
}

func newRespCache() *respCache {
	return &respCache{m: map[string]cacheEntry{}}
}

func (c *respCache) get(key string, now time.Time) (int, []byte, string, string, bool) {
	if key == "" {
		return 0, nil, "", "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || now.After(e.exp) {
		delete(c.m, key)
		return 0, nil, "", "", false
	}
	return e.status, e.body, e.etag, e.cc, true
}

func (c *respCache) put(key string, status int, body []byte, cc string, exp time.Time) {
	if key == "" || exp.IsZero() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) > 2048 {
		c.m = map[string]cacheEntry{}
	}
	c.m[key] = cacheEntry{status: status, body: append([]byte(nil), body...), etag: etagOf(body), cc: cc, exp: exp}
}

// sha256Hex is a tiny wrapper so server.go does not import crypto/sha256 twice with media.go.
// Server listens on the configured address.
type Server struct {
	http *http.Server
	cfg  Config
}

func NewServer(cfg Config, cat Catalog) *Server {
	h := NewHandler(cfg, cat)
	return &Server{
		cfg: h.cfg,
		http: &http.Server{
			Addr:              h.cfg.Listen,
			Handler:           h,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}
}

func (s *Server) ListenAndServe() error {
	log.Printf("linkpreview listening on %s", s.cfg.Listen)
	return s.http.ListenAndServe()
}
