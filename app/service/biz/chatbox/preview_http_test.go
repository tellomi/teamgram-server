package chatbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func previewKey(t *testing.T) ([]byte, string) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key, base64.StdEncoding.EncodeToString(key)
}

func stubOpenGraph(fn func(context.Context, *url.URL) fetchResult) func() {
	prev := loadOpenGraph
	loadOpenGraph = fn
	return func() { loadOpenGraph = prev }
}

func TestLinkPreviewEndpoint(t *testing.T) {
	var calls int
	restore := stubOpenGraph(func(_ context.Context, u *url.URL) fetchResult {
		calls++
		if u.String() != "https://example.com/post" {
			t.Fatalf("fetched %s", u)
		}
		return fetchResult{meta: pageMeta{
			Title: "Hello", Description: "From the page", Site: "Example",
			ImageURL: "https://cdn.example/pic.jpg",
		}}
	})
	defer restore()

	key, token := previewKey(t)
	h := PreviewHandler{Key: key}
	rec := postPreview(t, h, token, `{"url":"https://example.com/post"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var card previewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &card); err != nil {
		t.Fatal(err)
	}
	if card.URL != "https://example.com/post" || card.Title != "Hello" || card.Description != "From the page" || card.Image != "https://cdn.example/pic.jpg" || card.SiteName != "Example" {
		t.Fatalf("%+v", card)
	}
	if calls != 1 {
		t.Fatalf("calls %d", calls)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(rec.Header().Get("Cache-Control"))
	}

	rec = postPreview(t, h, "", `{"url":"https://example.com/post"}`)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"unauthorized"`) {
		t.Fatalf("auth %d %s", rec.Code, rec.Body.String())
	}
	rec = postPreview(t, h, base64.StdEncoding.EncodeToString([]byte("short")), `{"url":"https://example.com/post"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key %d", rec.Code)
	}

	rec = postPreview(t, h, token, `{"url":"http://127.0.0.1/secret"}`)
	if rec.Code != http.StatusBadRequest || calls != 1 {
		t.Fatalf("ssrf %d calls %d %s", rec.Code, calls, rec.Body.String())
	}
	rec = postPreview(t, h, token, `{"url":"http://169.254.169.254/latest"}`)
	if rec.Code != http.StatusBadRequest || calls != 1 {
		t.Fatalf("link-local %d calls %d", rec.Code, calls)
	}
	rec = postPreview(t, h, token, `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("json %d", rec.Code)
	}

	loadOpenGraph = func(context.Context, *url.URL) fetchResult {
		calls++
		return fetchResult{failed: true, pending: true}
	}
	rec = postPreview(t, h, token, `{"url":"https://example.com/slow"}`)
	if rec.Code != http.StatusGatewayTimeout || !strings.Contains(rec.Body.String(), `"timeout"`) {
		t.Fatalf("timeout %d %s", rec.Code, rec.Body.String())
	}

	loadOpenGraph = func(context.Context, *url.URL) fetchResult {
		return fetchResult{failed: true}
	}
	rec = postPreview(t, h, token, `{"url":"https://example.com/down"}`)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), `"fetch"`) {
		t.Fatalf("fetch %d %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/link-preview", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/internal/v1/other", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("path %d", rec.Code)
	}
}

func TestOpenGraphHostFallbackAndPrivateImage(t *testing.T) {
	restore := stubOpenGraph(func(context.Context, *url.URL) fetchResult {
		return fetchResult{meta: pageMeta{ImageURL: "https://cdn.example/a.jpg"}}
	})
	defer restore()
	card, err := OpenGraph(context.Background(), "https://example.com/x")
	if err != nil || card.Title != "example.com" || card.Site != "example.com" || card.Image != "https://cdn.example/a.jpg" {
		t.Fatalf("%+v %v", card, err)
	}
	if safeImageURL(context.Background(), "http://127.0.0.1/pic.jpg") != "" {
		t.Fatal("loopback image was kept")
	}
	if safeImageURL(context.Background(), "http://10.1.2.3/pic.jpg") != "" {
		t.Fatal("private image was kept")
	}
	if safeImageURL(context.Background(), "http://169.254.169.254/latest/meta-data") != "" {
		t.Fatal("link-local image was kept")
	}
	if got := safeImageURL(context.Background(), "http://1.1.1.1/pic.jpg"); got != "http://1.1.1.1/pic.jpg" {
		t.Fatalf("public image %q", got)
	}
	if safeImageURL(context.Background(), "http://localhost/pic.jpg") != "" {
		t.Fatal("localhost image was kept")
	}
}

func postPreview(t *testing.T, h PreviewHandler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/link-preview", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
