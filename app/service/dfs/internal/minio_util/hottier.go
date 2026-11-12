package minio_util

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	EnvHotTierEnabled      = "TELLOMI_MEDIA_HOT_TIER_ENABLED"
	EnvHotTierDir          = "TELLOMI_MEDIA_HOT_TIER_DIR"
	EnvHotTierMaxAge       = "TELLOMI_MEDIA_HOT_TIER_MAX_AGE"
	EnvHotTierUsedPct7d    = "TELLOMI_MEDIA_HOT_TIER_USED_PCT_7D"
	EnvHotTierUsedPct1d    = "TELLOMI_MEDIA_HOT_TIER_USED_PCT_1D"
	EnvHotTierUsedPct12h   = "TELLOMI_MEDIA_HOT_TIER_USED_PCT_12H"
	EnvHotTierUsedPct1h    = "TELLOMI_MEDIA_HOT_TIER_USED_PCT_1H"
	EnvHotTierUsedPctEmerg = "TELLOMI_MEDIA_HOT_TIER_USED_PCT_EMERGENCY"
	EnvHotTierInterval     = "TELLOMI_MEDIA_HOT_TIER_INTERVAL"
	EnvHotTierQuiet        = "TELLOMI_MEDIA_HOT_TIER_QUIET"

	defaultHotMaxAge   = 7 * 24 * time.Hour
	defaultHotInterval = time.Minute
	defaultHotQuiet    = 2 * time.Minute
	defaultPct7d       = 60
	defaultPct1d       = 75
	defaultPct12h      = 85
	defaultPct1h       = 90
	defaultPctEmerg    = 90
)

// hotSettings is loaded from the environment. Unexported so go-zero does
// not try to read it from yaml (duplicate json:"-" keys fail to load).
type hotSettings struct {
	Enabled      bool
	Dir          string
	MaxAge       time.Duration
	UsedPct7d    int
	UsedPct1d    int
	UsedPct12h   int
	UsedPct1h    int
	UsedPctEmerg int
	Interval     time.Duration
	Quiet        time.Duration
	// noLoop keeps NewMinioClient from starting the background migrator.
	// Tests call MigrateOnce themselves.
	noLoop    bool
	diskUsage func(path string) (used, total uint64, err error)
	now       func() time.Time
}

func defaultHotSettings() hotSettings {
	return hotSettings{
		MaxAge:       defaultHotMaxAge,
		UsedPct7d:    defaultPct7d,
		UsedPct1d:    defaultPct1d,
		UsedPct12h:   defaultPct12h,
		UsedPct1h:    defaultPct1h,
		UsedPctEmerg: defaultPctEmerg,
		Interval:     defaultHotInterval,
		Quiet:        defaultHotQuiet,
	}
}

func applyHotDefaults(h *hotSettings) {
	if h == nil {
		return
	}
	if h.MaxAge == 0 {
		h.MaxAge = defaultHotMaxAge
	}
	if h.Interval == 0 {
		h.Interval = defaultHotInterval
	}
	if h.Quiet == 0 {
		h.Quiet = defaultHotQuiet
	}
	if h.UsedPct7d == 0 && h.UsedPct1d == 0 && h.UsedPct12h == 0 && h.UsedPct1h == 0 && h.UsedPctEmerg == 0 {
		h.UsedPct7d = defaultPct7d
		h.UsedPct1d = defaultPct1d
		h.UsedPct12h = defaultPct12h
		h.UsedPct1h = defaultPct1h
		h.UsedPctEmerg = defaultPctEmerg
	}
}

func loadHotSettingsFromEnv(h *hotSettings) error {
	if h == nil {
		return fmt.Errorf("hot tier settings are nil")
	}
	applyHotDefaults(h)
	if v, ok := os.LookupEnv(EnvHotTierEnabled); ok && strings.TrimSpace(v) != "" {
		b, err := parseBool(v)
		if err != nil {
			return fmt.Errorf("%s: %w", EnvHotTierEnabled, err)
		}
		h.Enabled = b
	}
	if v, ok := os.LookupEnv(EnvHotTierDir); ok {
		h.Dir = strings.TrimSpace(v)
	}
	if err := optionalDuration(EnvHotTierMaxAge, &h.MaxAge); err != nil {
		return err
	}
	if err := optionalDuration(EnvHotTierInterval, &h.Interval); err != nil {
		return err
	}
	if err := optionalDuration(EnvHotTierQuiet, &h.Quiet); err != nil {
		return err
	}
	if err := optionalPct(EnvHotTierUsedPct7d, &h.UsedPct7d); err != nil {
		return err
	}
	if err := optionalPct(EnvHotTierUsedPct1d, &h.UsedPct1d); err != nil {
		return err
	}
	if err := optionalPct(EnvHotTierUsedPct12h, &h.UsedPct12h); err != nil {
		return err
	}
	if err := optionalPct(EnvHotTierUsedPct1h, &h.UsedPct1h); err != nil {
		return err
	}
	if err := optionalPct(EnvHotTierUsedPctEmerg, &h.UsedPctEmerg); err != nil {
		return err
	}
	return nil
}

func optionalDuration(env string, dst *time.Duration) error {
	v, ok := os.LookupEnv(env)
	if !ok || strings.TrimSpace(v) == "" {
		return nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil || d < 0 {
		return fmt.Errorf("%s: invalid duration %q", env, v)
	}
	*dst = d
	return nil
}

func optionalPct(env string, dst *int) error {
	v, ok := os.LookupEnv(env)
	if !ok || strings.TrimSpace(v) == "" {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 || n > 100 {
		return fmt.Errorf("%s: want an integer 0–100, got %q", env, v)
	}
	*dst = n
	return nil
}

// validateHotTier is called before the S3 client is built so a missing
// proxy cannot reach verifyBuckets over a direct dial.
func validateHotTier(h hotSettings, httpProxy string) error {
	if !h.Enabled {
		return nil
	}
	applyHotDefaults(&h)
	if strings.TrimSpace(h.Dir) == "" {
		return fmt.Errorf("%s is on, but %s is empty", EnvHotTierEnabled, EnvHotTierDir)
	}
	if strings.TrimSpace(httpProxy) == "" {
		return fmt.Errorf("%s is on, but %s is empty; the hot tier refuses to dial object storage directly", EnvHotTierEnabled, EnvHTTPProxy)
	}
	if _, err := parseObjectProxy(httpProxy); err != nil {
		return fmt.Errorf("%s is on, but %s is invalid: %w", EnvHotTierEnabled, EnvHTTPProxy, err)
	}
	if h.MaxAge <= 0 {
		return fmt.Errorf("%s must be greater than 0", EnvHotTierMaxAge)
	}
	if h.Interval <= 0 {
		return fmt.Errorf("%s must be greater than 0", EnvHotTierInterval)
	}
	if h.Quiet < 0 {
		return fmt.Errorf("%s must not be negative", EnvHotTierQuiet)
	}
	if h.UsedPct7d < 0 || h.UsedPct1d < h.UsedPct7d || h.UsedPct12h < h.UsedPct1d || h.UsedPct1h < h.UsedPct12h || h.UsedPctEmerg < h.UsedPct1h || h.UsedPctEmerg > 100 {
		return fmt.Errorf("hot tier used-percent thresholds must be non-decreasing in 0–100: 7d=%d 1d=%d 12h=%d 1h=%d emergency=%d",
			h.UsedPct7d, h.UsedPct1d, h.UsedPct12h, h.UsedPct1h, h.UsedPctEmerg)
	}
	return nil
}

func hotChatClass(class string) bool {
	switch class {
	case ClassDocuments, ClassPhotos, ClassVideos, ClassEncryptedFiles:
		return true
	default:
		return false
	}
}

type hotMeta struct {
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

type hotTier struct {
	dir        string
	maxAge     time.Duration
	pct7d      int
	pct1d      int
	pct12h     int
	pct1h      int
	pctEmerg   int
	interval   time.Duration
	quiet      time.Duration
	diskUsage  func(path string) (used, total uint64, err error)
	now        func() time.Time
	putRemote  func(ctx context.Context, class, path string, r io.Reader, size int64, contentType string) (minio.UploadInfo, error)
	getRemote  func(ctx context.Context, class, path string, offset int64, limit int32) ([]byte, error)
	statRemote func(ctx context.Context, class, path string) (int64, error)

	stop    context.CancelFunc
	stopped sync.WaitGroup
}

func newHotTier(h hotSettings, m *MinioUtil) (*hotTier, error) {
	if err := os.MkdirAll(h.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("hot tier directory: %w", err)
	}
	now := h.now
	if now == nil {
		now = time.Now
	}
	usage := h.diskUsage
	if usage == nil {
		usage = statfsUsage
	}
	t := &hotTier{
		dir:       h.Dir,
		maxAge:    h.MaxAge,
		pct7d:     h.UsedPct7d,
		pct1d:     h.UsedPct1d,
		pct12h:    h.UsedPct12h,
		pct1h:     h.UsedPct1h,
		pctEmerg:  h.UsedPctEmerg,
		interval:  h.Interval,
		quiet:     h.Quiet,
		diskUsage: usage,
		now:       now,
	}
	if m != nil {
		t.putRemote = m.putRemote
		t.getRemote = m.getRangeUncachedRemote
		t.statRemote = m.statRemote
	}
	return t, nil
}

func (t *hotTier) start() {
	if t == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.stop = cancel
	t.stopped.Add(1)
	go func() {
		defer t.stopped.Done()
		ticker := time.NewTicker(t.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := t.MigrateOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
					logx.Errorf("hot tier migrate: %v", err)
				}
			}
		}
	}()
}

func (t *hotTier) close() {
	if t == nil || t.stop == nil {
		return
	}
	t.stop()
	t.stopped.Wait()
}

func statfsUsage(path string) (used, total uint64, err error) {
	var st syscall.Statfs_t
	if err = syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bs := uint64(st.Bsize)
	total = st.Blocks * bs
	free := st.Bavail * bs
	if total < free {
		return 0, total, nil
	}
	return total - free, total, nil
}

// retention is how long a local object may stay before it is migrated.
// Each threshold is the used percent at which the window shortens:
//
//	< UsedPct7d: MaxAge (default 7 days)
//	>= UsedPct7d: 1 day
//	>= UsedPct1d: 12 hours
//	>= UsedPct12h: 1 hour
//	>= UsedPctEmerg: immediate (UsedPct1h, if higher, is treated as emergency too)
func (t *hotTier) retention(used, total uint64) time.Duration {
	if total == 0 {
		return t.maxAge
	}
	pct := int((used * 100) / total)
	emerg := t.pctEmerg
	if t.pct1h > emerg {
		emerg = t.pct1h
	}
	switch {
	case pct >= emerg:
		return 0
	case pct >= t.pct12h:
		return time.Hour
	case pct >= t.pct1d:
		return 12 * time.Hour
	case pct >= t.pct7d:
		return 24 * time.Hour
	default:
		return t.maxAge
	}
}

func (t *hotTier) localPath(class, path string) (string, error) {
	if !hotChatClass(class) {
		return "", fmt.Errorf("hot tier does not store class %s", class)
	}
	if path == "" || path != filepath.ToSlash(path) && strings.Contains(path, "\\") {
		// path is a logical object path using slashes
	}
	if path == "" || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("hot tier: invalid path %q", path)
	}
	full := filepath.Join(t.dir, class, filepath.FromSlash(path))
	root, err := filepath.Abs(t.dir)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("hot tier path escapes directory")
	}
	return abs, nil
}

func metaPath(p string) string { return p + ".meta" }

func isMetaOrPart(name string) bool {
	return strings.HasSuffix(name, ".meta") || strings.HasSuffix(name, ".part")
}

func (t *hotTier) write(class, path string, r io.Reader, size int64) (hotMeta, error) {
	var zero hotMeta
	full, err := t.localPath(class, path)
	if err != nil {
		return zero, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return zero, err
	}
	part := full + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return zero, err
	}
	defer func() {
		if part != "" {
			_ = os.Remove(part)
		}
	}()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return zero, err
	}
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, sum), r)
	if err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
		return zero, err
	}
	if size >= 0 && n != size {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
		return zero, fmt.Errorf("hot tier write size %d, want %d", n, size)
	}
	if err := f.Sync(); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
		return zero, err
	}
	if err := f.Close(); err != nil {
		return zero, err
	}
	meta := hotMeta{
		SHA256:    hex.EncodeToString(sum.Sum(nil)),
		Size:      n,
		CreatedAt: t.now().UTC(),
	}
	blob, err := json.Marshal(meta)
	if err != nil {
		return zero, err
	}
	mp := metaPath(full)
	if err := os.WriteFile(mp+".part", blob, 0o600); err != nil {
		return zero, err
	}
	if err := os.Rename(mp+".part", mp); err != nil {
		return zero, err
	}
	if err := os.Rename(part, full); err != nil {
		return zero, err
	}
	part = "" // skip defer remove of the successful data file
	return meta, nil
}

func (t *hotTier) loadMeta(full string) (hotMeta, error) {
	var meta hotMeta
	b, err := os.ReadFile(metaPath(full))
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		return meta, err
	}
	return meta, nil
}

func (t *hotTier) has(class, path string) bool {
	full, err := t.localPath(class, path)
	if err != nil {
		return false
	}
	st, err := os.Stat(full)
	return err == nil && st.Mode().IsRegular()
}

func (t *hotTier) open(class, path string) (*os.File, hotMeta, error) {
	var zero hotMeta
	full, err := t.localPath(class, path)
	if err != nil {
		return nil, zero, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, zero, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		f.Close()
		return nil, zero, err
	}
	meta, err := t.loadMeta(full)
	if err != nil {
		// A file without meta is still readable; size comes from the file.
		st, sterr := f.Stat()
		if sterr != nil {
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			f.Close()
			return nil, zero, sterr
		}
		meta.Size = st.Size()
		return f, meta, nil
	}
	return f, meta, nil
}

func (t *hotTier) readRange(class, path string, offset int64, limit int32) ([]byte, error) {
	f, meta, err := t.open(class, path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}()
	if offset < 0 {
		return nil, fmt.Errorf("invalid offset %d", offset)
	}
	if offset > meta.Size {
		return nil, ErrNotFound
	}
	if limit < 0 {
		return nil, fmt.Errorf("invalid read limit %d", limit)
	}
	if limit == 0 {
		return []byte{}, nil
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	remain := meta.Size - offset
	n := int64(limit)
	if n > remain {
		n = remain
	}
	buf := make([]byte, n)
	got, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	return buf[:got], nil
}

func (t *hotTier) remove(class, path string) error {
	full, err := t.localPath(class, path)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(metaPath(full)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (t *hotTier) moveToTrash(class, path string) error {
	full, err := t.localPath(class, path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(full); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	dest := filepath.Join(t.dir, ".trash", class, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	if err := os.Rename(full, dest); err != nil {
		return err
	}
	_ = os.Rename(metaPath(full), metaPath(dest))
	return nil
}

func (t *hotTier) deleteTrash(class, path string) error {
	dest := filepath.Join(t.dir, ".trash", class, filepath.FromSlash(path))
	root, err := filepath.Abs(filepath.Join(t.dir, ".trash"))
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("hot tier trash path escapes directory")
	}
	if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(metaPath(abs)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// MigrateOnce moves local objects that have exceeded the current retention
// to R2, verifies size and sha256, then deletes the local copy. A failed
// verification leaves the local file in place.
func (t *hotTier) MigrateOnce(ctx context.Context) error {
	if t == nil {
		return nil
	}
	used, total, err := t.diskUsage(t.dir)
	if err != nil {
		return fmt.Errorf("hot tier disk usage: %w", err)
	}
	keep := t.retention(used, total)
	now := t.now()
	return filepath.WalkDir(t.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".trash" && path == filepath.Join(t.dir, ".trash") {
				return filepath.SkipDir
			}
			return nil
		}
		if isMetaOrPart(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(t.dir, path)
		if err != nil {
			return nil
		}
		class, rest, ok := strings.Cut(filepath.ToSlash(rel), "/")
		if !ok || !hotChatClass(class) {
			return nil
		}
		return t.migrateFile(ctx, class, rest, path, keep, now)
	})
}

func (t *hotTier) migrateFile(ctx context.Context, class, path, full string, keep time.Duration, now time.Time) error {
	st, err := os.Stat(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if now.Sub(st.ModTime()) < t.quiet {
		return nil
	}
	meta, err := t.loadMeta(full)
	created := st.ModTime()
	if err == nil && !meta.CreatedAt.IsZero() {
		created = meta.CreatedAt
	}
	if keep > 0 && now.Sub(created) < keep {
		return nil
	}
	f, err := os.Open(full)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil // being read or written; try later
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	sum := sha256.New()
	n, err := io.Copy(sum, f)
	if err != nil {
		return err
	}
	localHash := hex.EncodeToString(sum.Sum(nil))
	if meta.SHA256 != "" && (meta.SHA256 != localHash || meta.Size != n) {
		return fmt.Errorf("hot tier %s/%s local sha256 or size does not match meta; leaving file in place", class, path)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if t.putRemote == nil {
		return fmt.Errorf("hot tier has no remote put")
	}
	if _, err := t.putRemote(ctx, class, path, f, n, contentTypeFor(path)); err != nil {
		logx.WithContext(ctx).Errorf("hot tier put %s/%s: %v", class, path, err)
		return nil // leave local, retry later
	}
	if t.getRemote == nil {
		return fmt.Errorf("hot tier has no remote get")
	}
	got, err := t.getRemote(ctx, class, path, 0, int32(n))
	if err != nil {
		logx.WithContext(ctx).Errorf("hot tier verify get %s/%s: %v", class, path, err)
		return nil
	}
	remoteHash := sha256.Sum256(got)
	if int64(len(got)) != n || hex.EncodeToString(remoteHash[:]) != localHash {
		logx.WithContext(ctx).Errorf("hot tier verify mismatch %s/%s; local copy kept", class, path)
		return nil
	}
	// Close and unlock before unlink so we are not unlinking an open fd
	// that another reader might still want. The exclusive lock was held
	// through the verify, so no reader had the file.
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
	if err := os.Remove(full); err != nil {
		return err
	}
	_ = os.Remove(metaPath(full))
	return nil
}
