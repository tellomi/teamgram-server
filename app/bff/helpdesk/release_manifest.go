package helpdesk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// defaultManifestURL is the public Android release feed. Chengdu syncs
	// the same object from the R2 bucket tellomi-updates.
	defaultManifestURL = "https://www.tellomi.cn/dl/android/latest.json"
	manifestMaxBytes   = 1 << 20
)

// localManifestCandidates are host or container paths a sync job may write.
// The first regular file wins. Otherwise LoadRelease fetches defaultManifestURL.
var localManifestCandidates = []string{
	"/etc/tellomi/updates/android/latest.json",
	"/opt/tellomi/updates/android/latest.json",
	"/var/www/tellomi/dl/android/latest.json",
	"/usr/share/nginx/html/dl/android/latest.json",
}

var (
	manifestSuccessTTL  = 5 * time.Minute
	manifestFailureTTL  = 30 * time.Second
	manifestHTTPTimeout = 5 * time.Second
	manifestNow         = time.Now
	manifestHTTP        = &http.Client{}
)

type manifestEntry struct {
	until time.Time
	rel   *Release
	err   error
}

var (
	manifestMu     sync.Mutex
	manifestCache  = map[string]manifestEntry{}
	manifestFlight sync.Map // source -> *manifestCall
)

type manifestCall struct {
	done chan struct{}
	rel  *Release
	err  error
}

// publishedManifest reads the configured Android release feed.
// A disabled feed returns nil, nil. A read or parse error is cached
// briefly so a dead feed does not add a timeout to every help.getAppUpdate.
func publishedManifest() (*Release, error) {
	source, ok := resolveManifestSource()
	if !ok {
		return nil, nil
	}
	return loadCachedManifest(source)
}

// resolveManifestSource picks the feed.
// TELLOMI_APP_UPDATE_MANIFEST is a file path or an http(s) URL.
// Empty (including the empty string Compose injects) uses the first local
// candidate that exists, then defaultManifestURL.
// off, none, and "-" disable the feed.
func resolveManifestSource() (string, bool) {
	raw, set := os.LookupEnv("TELLOMI_APP_UPDATE_MANIFEST")
	return pickManifestSource(raw, set, localManifestCandidates)
}

func pickManifestSource(raw string, set bool, candidates []string) (string, bool) {
	if set {
		raw = strings.TrimSpace(raw)
		switch strings.ToLower(raw) {
		case "":
		case "off", "none", "-":
			return "", false
		default:
			return raw, true
		}
	}
	for _, path := range candidates {
		st, err := os.Stat(path)
		if err == nil && st.Mode().IsRegular() {
			return path, true
		}
	}
	return defaultManifestURL, true
}

func loadCachedManifest(source string) (*Release, error) {
	if rel, err, ok := cachedManifest(source); ok {
		return rel, err
	}
	call := &manifestCall{done: make(chan struct{})}
	actual, loaded := manifestFlight.LoadOrStore(source, call)
	if loaded {
		existing := actual.(*manifestCall)
		<-existing.done
		return cloneRelease(existing.rel), existing.err
	}
	rel, err := readManifest(context.Background(), source)
	if err != nil {
		logx.Errorf("help.getAppUpdate: release manifest %q: %v", source, err)
	} else if rel != nil {
		logx.Infof("help.getAppUpdate: release manifest %q version %s", source, rel.Version)
	}
	ttl := manifestSuccessTTL
	if err != nil {
		ttl = manifestFailureTTL
	}
	storeManifest(source, rel, err, ttl)
	call.rel = rel
	call.err = err
	close(call.done)
	manifestFlight.Delete(source)
	return cloneRelease(rel), err
}

func cachedManifest(source string) (*Release, error, bool) {
	manifestMu.Lock()
	defer manifestMu.Unlock()
	entry, ok := manifestCache[source]
	if !ok || !manifestNow().Before(entry.until) {
		return nil, nil, false
	}
	return cloneRelease(entry.rel), entry.err, true
}

func storeManifest(source string, rel *Release, err error, ttl time.Duration) {
	manifestMu.Lock()
	defer manifestMu.Unlock()
	manifestCache[source] = manifestEntry{
		until: manifestNow().Add(ttl),
		rel:   cloneRelease(rel),
		err:   err,
	}
}

func cloneRelease(rel *Release) *Release {
	if rel == nil {
		return nil
	}
	out := *rel
	return &out
}

func readManifest(ctx context.Context, source string) (*Release, error) {
	body, err := readManifestBody(ctx, source)
	if err != nil {
		return nil, err
	}
	return parseManifest(body)
}

func readManifestBody(ctx context.Context, source string) ([]byte, error) {
	if strings.Contains(source, "://") {
		parsed, err := url.Parse(source)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https":
			return fetchManifest(ctx, source)
		default:
			return nil, fmt.Errorf("unsupported manifest scheme %q", parsed.Scheme)
		}
	}
	return readManifestFile(source)
}

func readManifestFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("manifest %s is not a file", path)
	}
	if st.Size() > manifestMaxBytes {
		return nil, fmt.Errorf("manifest %s is larger than %d bytes", path, manifestMaxBytes)
	}
	return readLimited(f)
}

func fetchManifest(ctx context.Context, rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, manifestHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "tellomi-bff help.getAppUpdate")
	resp, err := manifestHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest HTTP %s", resp.Status)
	}
	return readLimited(resp.Body)
}

func readLimited(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, manifestMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > manifestMaxBytes {
		return nil, fmt.Errorf("manifest larger than %d bytes", manifestMaxBytes)
	}
	return body, nil
}

// androidManifest is the published /dl/android/latest.json object.
// versionName is what help.getAppUpdate compares. url is the button link,
// and fallbackUrl is used when url is empty. changelog, when it is a string,
// is the dialog text. Other keys (versionCode, sha256, sha256sum, size, files)
// are accepted and ignored.
type androidManifest struct {
	VersionName string          `json:"versionName"`
	URL         string          `json:"url"`
	FallbackURL string          `json:"fallbackUrl"`
	Changelog   json.RawMessage `json:"changelog"`
}

func parseManifest(body []byte) (*Release, error) {
	var doc androidManifest
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	version := strings.TrimSpace(doc.VersionName)
	if version == "" {
		return nil, fmt.Errorf("manifest versionName is empty")
	}
	link := strings.TrimSpace(doc.URL)
	if link == "" {
		link = strings.TrimSpace(doc.FallbackURL)
	}
	return &Release{
		Version:    version,
		Text:       manifestChangelog(doc.Changelog),
		URL:        link,
		CannotSkip: true,
	}, nil
}

func manifestChangelog(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	if raw[0] != '"' {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}
