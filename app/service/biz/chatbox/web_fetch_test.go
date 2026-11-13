package chatbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/alicebob/miniredis/v2"
	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"golang.org/x/text/encoding/simplifiedchinese"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func stubWebFetch(t *testing.T, meta pageMeta) func() {
	t.Helper()
	t.Setenv(EnvServerLinkFetch, "1")
	prev := loadWebPage
	loadWebPage = func(context.Context, *url.URL) fetchResult { return fetchResult{meta: meta} }
	return func() { loadWebPage = prev }
}

func TestBlockedWebIPs(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.1.2.3", "192.168.0.5", "172.16.4.4", "169.254.169.254", "::1", "100.64.0.1"} {
		if !blockedIP(net.ParseIP(raw)) {
			t.Fatalf("%s was allowed", raw)
		}
	}
	if blockedIP(net.ParseIP("1.1.1.1")) || blockedIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public address was blocked")
	}
	if _, err := parseWebURL("http://127.0.0.1/secret"); err == nil {
		t.Fatal("loopback URL was accepted")
	}
	if _, err := parseWebURL("http://169.254.169.254/latest"); err == nil {
		t.Fatal("link-local URL was accepted")
	}
	for _, raw := range []string{
		"http://[::1]/secret",
		"http://[fe80::1]/",
		"http://[fd00::1]/",
		"http://[::ffff:127.0.0.1]/",
		"http://[::ffff:10.1.2.3]/",
		"http://[::ffff:169.254.169.254]/",
		"http://localhost/secret",
		"http://app.localhost/secret",
	} {
		if _, err := parseWebURL(raw); err == nil {
			t.Fatalf("%s was accepted", raw)
		}
	}
	if _, err := parseWebURL("http://[2606:4700:4700::1111]/"); err != nil {
		t.Fatalf("public ipv6: %v", err)
	}
}

func TestOrdinaryPhotoDoesNotReadPersistedPreview(t *testing.T) {
	var probed atomic.Int32
	SetWebPhotoReaderForTest(func(string) ([]byte, error) {
		probed.Add(1)
		return []byte{0xff, 0xd8}, nil
	})
	t.Cleanup(func() { SetWebPhotoReaderForTest(nil) })

	const id int64 = 2121303527006408704
	if _, _, ok := WebPhotoForDownload(id, id+1); ok || probed.Load() != 0 {
		t.Fatalf("ordinary photo ok, probed %d", probed.Load())
	}
	body, _, ok := WebPhotoForDownload(id, id)
	if !ok || len(body) == 0 || probed.Load() != 1 {
		t.Fatalf("preview ok %v probed %d", ok, probed.Load())
	}
	if _, _, ok = WebPhotoForDownload(id, id); !ok || probed.Load() != 1 {
		t.Fatalf("second read probed %d", probed.Load())
	}
}

func TestOpenGraphAndPhoto(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 50}); err != nil {
		t.Fatal(err)
	}
	html := `<html><head>
<meta property="og:title" content="Hello">
<meta property="og:description" content="From the page">
<meta property="og:site_name" content="Example">
<meta property="og:image" content="/pic.jpg">
<title>Other</title>
</head></html>`
	base, _ := url.Parse("https://example.com/post")
	meta := parseMeta(html, base)
	if meta.Title != "Hello" || meta.Description != "From the page" || meta.Site != "Example" || meta.ImageURL != "https://example.com/pic.jpg" {
		t.Fatalf("meta %#v", meta)
	}
	restore := stubWebFetch(t, pageMeta{
		Title: "Hello", Description: "From the page", Site: "Example",
		Image: buf.Bytes(), ImageMIME: "image/jpeg",
	})
	defer restore()
	page, err := WebPage("https://example.com/og-card", 0)
	if err != nil || page.GetPredicateName() != mtproto.Predicate_webPage {
		t.Fatal(err, page.GetPredicateName())
	}
	if page.GetPhoto() == nil || page.GetPhoto().GetDcId() == 0 || len(page.GetPhoto().GetSizes()) == 0 {
		t.Fatal("og:image was not stored as a photo")
	}
	body, mime, ok := WebPhoto(page.GetPhoto().GetId())
	if !ok || mime != "image/jpeg" || len(body) == 0 {
		t.Fatal("photo bytes missing")
	}
}

func TestTallVideoPreviewKeepsAspect(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 12, 20))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 40}); err != nil {
		t.Fatal(err)
	}
	restore := stubWebFetch(t, pageMeta{
		Title: "Tall", Image: buf.Bytes(), ImageMIME: "image/jpeg",
	})
	defer restore()
	page, err := WebPage("https://example.com/tall-cover", 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.GetType().GetValue() != "photo" {
		t.Fatalf("type %q", page.GetType().GetValue())
	}
	sz := page.GetPhoto().GetSizes()
	if len(sz) == 0 || sz[0].GetW() >= sz[0].GetH() {
		t.Fatalf("size %#v", sz)
	}

	square := image.NewRGBA(image.Rect(0, 0, 8, 8))
	buf.Reset()
	if err := jpeg.Encode(&buf, square, &jpeg.Options{Quality: 40}); err != nil {
		t.Fatal(err)
	}
	restore()
	restore = stubWebFetch(t, pageMeta{
		Title: "Douyin", OGType: "video.other", HasVideo: true,
		VideoW: 720, VideoH: 1280,
		Image: buf.Bytes(), ImageMIME: "image/jpeg",
	})
	defer restore()
	page, err = WebPage("https://v.douyin.com/example", 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.GetType().GetValue() != "video" {
		t.Fatalf("type %q", page.GetType().GetValue())
	}
	sz = page.GetPhoto().GetSizes()
	if len(sz) == 0 || sz[0].GetW() >= sz[0].GetH() || sz[0].GetW() == sz[0].GetH() {
		t.Fatalf("video card stored square %#v", sz)
	}
}

func TestOpenGraphVideoSize(t *testing.T) {
	html := `<meta property="og:type" content="video.other">
<meta property="og:video:width" content="720">
<meta property="og:video:height" content="1280">
<meta property="og:video" content="https://cdn.example/v.mp4">`
	meta := parseMeta(html, nil)
	if meta.OGType != "video.other" || !meta.HasVideo || meta.VideoW != 720 || meta.VideoH != 1280 {
		t.Fatalf("%#v", meta)
	}
	if previewType(meta, 8, 8) != "video" {
		t.Fatal(previewType(meta, 8, 8))
	}
}

func TestGBKTitle(t *testing.T) {
	raw, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("<title>中文标题</title>"))
	if err != nil {
		t.Fatal(err)
	}
	text := decodeHTML(raw, "text/html; charset=gbk")
	meta := parseMeta(text, nil)
	if meta.Title != "中文标题" {
		t.Fatalf("title %q", meta.Title)
	}
}

func TestFetchOpenGraphBlocksLoopbackAtDial(t *testing.T) {
	u, err := url.Parse("http://127.0.0.1/secret")
	if err != nil {
		t.Fatal(err)
	}
	res := fetchOpenGraph(context.Background(), u)
	if !res.failed || !res.blocked {
		t.Fatalf("%+v", res)
	}
	card, err := OpenGraph(context.Background(), "http://[::1]/secret")
	if !errors.Is(err, errPreviewDenied) || card.Title != "" {
		t.Fatalf("%+v %v", card, err)
	}
}

func TestFetchOpenGraphImageUsesFinalURL(t *testing.T) {
	land := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<meta property="og:title" content="Landed"><meta property="og:image" content="/pic.jpg">`)
	}))
	defer land.Close()
	hop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, land.URL+"/land", http.StatusFound)
	}))
	defer hop.Close()

	prevClient := webHTTPClient
	prevFilter := filterImageURL
	prevVet := vetRequest
	webHTTPClient = func() *http.Client { return &http.Client{Timeout: webTimeout} }
	vetRequest = func(context.Context, *url.URL) error { return nil }
	// The land host is loopback. Keep the resolved URL so the assertion can
	// see it; production drops that address in safeImageURL.
	var resolved string
	filterImageURL = func(_ context.Context, raw string) string {
		resolved = raw
		return ""
	}
	defer func() {
		webHTTPClient = prevClient
		filterImageURL = prevFilter
		vetRequest = prevVet
	}()

	u, err := url.Parse(hop.URL + "/go")
	if err != nil {
		t.Fatal(err)
	}
	res := fetchOpenGraph(context.Background(), u)
	if res.failed {
		t.Fatalf("%+v", res)
	}
	landURL, err := url.Parse(land.URL)
	if err != nil {
		t.Fatal(err)
	}
	want := landURL.Scheme + "://" + landURL.Host + "/pic.jpg"
	if res.meta.Title != "Landed" || resolved != want {
		t.Fatalf("title %q image %q want %s", res.meta.Title, resolved, want)
	}
}

func TestExternalFetchOffByDefault(t *testing.T) {
	t.Setenv(EnvServerLinkFetch, "")
	called := false
	prev := loadWebPage
	loadWebPage = func(context.Context, *url.URL) fetchResult {
		called = true
		return fetchResult{meta: pageMeta{Title: "Apple"}}
	}
	defer func() { loadWebPage = prev }()
	page, err := WebPage("https://www.apple.com/tellomi-fetch-off", 0)
	if err != nil || called || page.GetPredicateName() != mtproto.Predicate_webPageEmpty {
		t.Fatalf("page %s called %v err %v", page.GetPredicateName(), called, err)
	}
	preview := WebPagePreview("https://www.apple.com/tellomi-fetch-off-preview", nil)
	if preview.GetPredicateName() != mtproto.Predicate_messageMediaEmpty {
		t.Fatalf("preview %s", preview.GetPredicateName())
	}
}

func TestConcurrentFetchIsOneRequest(t *testing.T) {
	resetFetchLimits()
	t.Setenv(EnvServerLinkFetch, "1")
	var n atomic.Int32
	prev := loadWebPage
	loadWebPage = func(context.Context, *url.URL) fetchResult {
		n.Add(1)
		time.Sleep(40 * time.Millisecond)
		return fetchResult{meta: pageMeta{Title: "Once"}}
	}
	defer func() { loadWebPage = prev }()
	const raw = "https://cache-hit.example/card"
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			page, err := WebPage(raw, 0)
			if err != nil || page.GetTitle().GetValue() != "Once" {
				errCh <- fmt.Errorf("%v title %q", err, page.GetTitle().GetValue())
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	if n.Load() != 1 {
		t.Fatalf("fetches = %d, want 1", n.Load())
	}
}

func TestDomainRateLimitDoesNotFetch(t *testing.T) {
	resetFetchLimits()
	t.Setenv(EnvServerLinkFetch, "1")
	var n atomic.Int32
	prev := loadWebPage
	loadWebPage = func(context.Context, *url.URL) fetchResult {
		n.Add(1)
		return fetchResult{meta: pageMeta{Title: "Ok"}}
	}
	defer func() { loadWebPage = prev }()
	for i := 0; i < webDomainBurst; i++ {
		page, err := WebPage(fmt.Sprintf("https://rate-limit.example/p%d", i), 0)
		if err != nil || page.GetPredicateName() != mtproto.Predicate_webPage {
			t.Fatalf("call %d %s %v", i, page.GetPredicateName(), err)
		}
	}
	over, err := WebPage("https://rate-limit.example/over", 0)
	if err != nil || over.GetPredicateName() != mtproto.Predicate_webPageEmpty {
		t.Fatalf("over limit %s %v", over.GetPredicateName(), err)
	}
	if int(n.Load()) != webDomainBurst {
		t.Fatalf("fetches = %d, want %d", n.Load(), webDomainBurst)
	}
}

func TestNegativeCacheSkipsSecondFetch(t *testing.T) {
	resetFetchLimits()
	t.Setenv(EnvServerLinkFetch, "1")
	var n atomic.Int32
	prev := loadWebPage
	loadWebPage = func(context.Context, *url.URL) fetchResult {
		n.Add(1)
		return fetchResult{failed: true}
	}
	defer func() { loadWebPage = prev }()
	const raw = "https://negative-cache.example/missing"
	page, err := WebPage(raw, 0)
	if err != nil || page.GetPredicateName() != mtproto.Predicate_webPageEmpty {
		t.Fatalf("first %s %v", page.GetPredicateName(), err)
	}
	page, err = WebPage(raw, 0)
	if err != nil || page.GetPredicateName() != mtproto.Predicate_webPageEmpty || n.Load() != 1 {
		t.Fatalf("second fetches %d %s %v", n.Load(), page.GetPredicateName(), err)
	}
}

func TestProxyAndPortsDoNotFetchPrivateTargets(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	defer proxy.Close()
	t.Setenv(EnvLinkFetchProxy, proxy.URL)
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("http_proxy", "http://127.0.0.1:9")
	for _, raw := range []string{
		"http://127.0.0.1/secret",
		"http://10.1.2.3/a",
		"http://169.254.169.254/latest",
		"http://1.1.1.1:8080/",
		"https://example.com:8443/a",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		res := fetchOpenGraph(context.Background(), u)
		if !res.failed || !res.blocked {
			t.Fatalf("%s %+v", raw, res)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("proxy saw %d requests", hits.Load())
	}
	t.Setenv(EnvLinkFetchProxy, "")
	c := safeClient()
	tr, _ := c.Transport.(*http.Transport)
	if tr == nil || tr.Proxy != nil {
		req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
		if tr != nil && tr.Proxy != nil {
			got, err := tr.Proxy(req)
			if err != nil || got != nil {
				t.Fatalf("env proxy leaked: %v %v", got, err)
			}
		}
	}
}

func TestRedisPageCacheRoundTrip(t *testing.T) {
	mr := miniredis.RunT(t)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	prev := redisCli
	redisCli = rds
	t.Cleanup(func() { redisCli = prev })
	page := mtproto.MakeTLWebPage(&mtproto.WebPage{
		Id:         7,
		Url_STRING: "https://cache.example/a",
		Type:       wrappersString("article"),
		Title:      wrappersString("Cached"),
	}).To_WebPage()
	pageCacheSet(context.Background(), "abc", page, time.Hour)
	localPages = &memoryPages{}
	got, ok := pageCacheGet(context.Background(), "abc")
	if !ok || got.GetPredicateName() != mtproto.Predicate_webPage || got.GetTitle().GetValue() != "Cached" {
		t.Fatalf("redis page %#v ok %v", got, ok)
	}
}

func wrappersString(s string) *wrapperspb.StringValue {
	return wrapperspb.String(s)
}

func TestRedisGlobalSlotCap(t *testing.T) {
	mr := miniredis.RunT(t)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	prev := redisCli
	redisCli = rds
	t.Cleanup(func() { redisCli = prev })
	var releases []func()
	for i := 0; i < webGlobalSlots; i++ {
		release, decided, ok := acquireRedisSlot()
		if !decided || !ok || release == nil {
			t.Fatalf("slot %d decided %v ok %v", i, decided, ok)
		}
		releases = append(releases, release)
	}
	_, decided, ok := acquireRedisSlot()
	if !decided || ok {
		t.Fatalf("over cap decided %v ok %v", decided, ok)
	}
	releases[0]()
	_, decided, ok = acquireRedisSlot()
	if !decided || !ok {
		t.Fatalf("after release decided %v ok %v", decided, ok)
	}
}

func TestWebPhotoDedupSkipsSecondWrite(t *testing.T) {
	var writes atomic.Int32
	prevWrite := writeWebObject
	prevClaim := claimWebBlob
	seen := map[string]bool{}
	var mu sync.Mutex
	writeWebObject = func(string, []byte) error {
		writes.Add(1)
		return nil
	}
	claimWebBlob = func(sha string, size int64, proposed string) (string, bool) {
		mu.Lock()
		defer mu.Unlock()
		if seen[sha] {
			return proposed, true
		}
		seen[sha] = true
		return proposed, false
	}
	defer func() {
		writeWebObject = prevWrite
		claimWebBlob = prevClaim
	}()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 40}); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	a := storeWebPhoto(raw, "image/jpeg")
	b := storeWebPhoto(raw, "image/jpeg")
	if a == nil || b == nil || a.GetId() != b.GetId() || a.GetAccessHash() != a.GetId() {
		t.Fatalf("a %#v b %#v", a, b)
	}
	if writes.Load() != 1 {
		t.Fatalf("writes %d", writes.Load())
	}
	if strings.HasPrefix(webPhotoPath(a.GetId()), "0/") {
		t.Fatalf("preview path %s collides with a photo original", webPhotoPath(a.GetId()))
	}
}

func TestOneLineKeepsUTF8(t *testing.T) {
	s := strings.Repeat("a", 299) + "é" + "z"
	got := oneLine(s)
	if !utf8.ValidString(got) || got != strings.Repeat("a", 299) {
		t.Fatalf("cut %q", got)
	}
}

func TestWebPageFailureShapes(t *testing.T) {
	t.Setenv(EnvServerLinkFetch, "1")
	prev := loadWebPage
	defer func() { loadWebPage = prev }()
	loadWebPage = func(context.Context, *url.URL) fetchResult { return fetchResult{failed: true} }
	page, err := WebPage("https://example.com/missing-page", 0)
	if err != nil || page.GetPredicateName() != mtproto.Predicate_webPageEmpty {
		t.Fatalf("empty %+v %v", page, err)
	}
	loadWebPage = func(context.Context, *url.URL) fetchResult { return fetchResult{failed: true, pending: true} }
	page, err = WebPage("https://example.com/slow-page", 0)
	if err != nil || page.GetPredicateName() != mtproto.Predicate_webPagePending || page.GetDate() == 0 {
		t.Fatalf("pending %+v %v", page, err)
	}
}

func TestFetchRemoteHonorsDeadlineOnHangingResolver(t *testing.T) {
	t.Setenv(EnvServerLinkFetch, "1")

	ln, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	// The socket is never read or written, so DNS queries hang until ctx ends.

	prevResolver := webResolver
	webResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", ln.LocalAddr().String())
		},
	}
	prevLoad := loadWebPage
	loadWebPage = fetchRemote
	t.Cleanup(func() {
		webResolver = prevResolver
		loadWebPage = prevLoad
	})

	const deadline = 80 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	start := time.Now()
	page, _, _, err := WebPageContext(ctx, "http://tellomi-hanging-dns.invalid/preview", 0)
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("fetch hung past deadline: %s", elapsed)
	}
	if elapsed < deadline/2 {
		t.Fatalf("returned too fast (%s); hanging resolver may not have been used", elapsed)
	}
	if err != nil {
		t.Fatalf("WebPageContext: %v", err)
	}
	if page == nil || page.GetPredicateName() != mtproto.Predicate_webPageEmpty {
		got := "nil"
		if page != nil {
			got = page.GetPredicateName()
		}
		t.Fatalf("got %s, want webPageEmpty", got)
	}
}
