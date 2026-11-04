package chatbox

import (
	"bytes"
	"image"
	"image/jpeg"
	"net"
	"net/url"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func stubWebFetch(meta pageMeta) func() {
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
	restore := stubWebFetch(pageMeta{
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

func TestWebPageFailureShapes(t *testing.T) {
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
