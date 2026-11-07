package chatbox

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func stubWebFetch(t *testing.T, meta pageMeta) func() {
	t.Helper()
	t.Setenv(EnvServerLinkFetch, "1")
	prev := loadWebPage
	loadWebPage = func(*url.URL) fetchResult { return fetchResult{meta: meta} }
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
	webHTTPClient = func() *http.Client { return &http.Client{Timeout: webTimeout} }
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
	loadWebPage = func(*url.URL) fetchResult {
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
	loadWebPage = func(*url.URL) fetchResult { return fetchResult{failed: true} }
	page, err := WebPage("https://example.com/missing-page", 0)
	if err != nil || page.GetPredicateName() != mtproto.Predicate_webPageEmpty {
		t.Fatalf("empty %+v %v", page, err)
	}
	loadWebPage = func(*url.URL) fetchResult { return fetchResult{failed: true, pending: true} }
	page, err = WebPage("https://example.com/slow-page", 0)
	if err != nil || page.GetPredicateName() != mtproto.Predicate_webPagePending || page.GetDate() == 0 {
		t.Fatalf("pending %+v %v", page, err)
	}
}
