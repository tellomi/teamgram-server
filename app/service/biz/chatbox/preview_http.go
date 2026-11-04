package chatbox

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// EnvLinkPreview turns the listener on. Only the value 1 counts.
	// This is the content-engine card fetch. It is not the public
	// tell.cc resolver (TELLOMI_LINKPREVIEW_*, port 8801).
	EnvLinkPreview = "TELLOMI_LINK_PREVIEW"
	// EnvLinkPreviewKey is standard base64 of at least 32 bytes. The engine
	// sends that same base64 as Authorization: Bearer, the same scheme as
	// POST /internal/v1/ai/complete.
	EnvLinkPreviewKey = "TELLOMI_LINK_PREVIEW_KEY"
	// EnvLinkPreviewAddr is the listen address. Empty means 127.0.0.1:8817.
	// "off" does not listen.
	EnvLinkPreviewAddr = "TELLOMI_LINK_PREVIEW_ADDR"

	defaultLinkPreviewAddr = "127.0.0.1:8817"
	pathLinkPreview        = "/internal/v1/link-preview"
	maxPreviewBody         = 64 << 10
)

// StartLinkPreview listens for content-engine when TELLOMI_LINK_PREVIEW=1.
// A missing or short key is logged and does not stop bff. Unset does not listen.
// The HTTP deadlines match POST /internal/v1/ai/complete. The page fetch itself
// stops at webTimeout (8s).
func StartLinkPreview() {
	if os.Getenv(EnvLinkPreview) != "1" {
		return
	}
	key, err := decodePreviewKey(os.Getenv(EnvLinkPreviewKey))
	if err != nil {
		logx.Error("link preview: internal key is missing or invalid; engine endpoint is off")
		return
	}
	addr := os.Getenv(EnvLinkPreviewAddr)
	if addr == "" {
		addr = defaultLinkPreviewAddr
	}
	if addr == "off" {
		return
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           PreviewHandler{Key: key},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		logx.Infof("link preview listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Errorf("link preview http failed")
		}
	}()
}

func decodePreviewKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errPreviewDenied
	}
	buf, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		buf, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil || len(buf) < 32 {
		return nil, errPreviewDenied
	}
	return buf, nil
}

// PreviewHandler is POST /internal/v1/link-preview.
type PreviewHandler struct {
	Key []byte
}

type previewRequest struct {
	URL string `json:"url"`
}

type previewResponse struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
	SiteName    string `json:"site_name"`
}

// ServeHTTP authenticates the engine and returns one card.
func (h PreviewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != pathLinkPreview {
		writePreviewErr(w, http.StatusNotFound, "not_found")
		return
	}
	if r.Method != http.MethodPost {
		writePreviewErr(w, http.StatusMethodNotAllowed, "method")
		return
	}
	if !previewAuthorized(r, h.Key) {
		writePreviewErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body previewRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxPreviewBody)).Decode(&body); err != nil {
		writePreviewErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	card, err := OpenGraph(r.Context(), body.URL)
	if errors.Is(err, errPreviewDenied) {
		writePreviewErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if errors.Is(err, errPreviewTimeout) {
		writePreviewErr(w, http.StatusGatewayTimeout, "timeout")
		return
	}
	if err != nil {
		writePreviewErr(w, http.StatusBadGateway, "fetch")
		return
	}
	writePreviewJSON(w, http.StatusOK, previewResponse{
		URL:         card.URL,
		Title:       card.Title,
		Description: card.Description,
		Image:       card.Image,
		SiteName:    card.Site,
	})
}

func previewAuthorized(r *http.Request, key []byte) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || len(key) == 0 {
		return false
	}
	got, err := base64.StdEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil || len(got) != len(key) {
		return false
	}
	return hmac.Equal(got, key)
}

func writePreviewErr(w http.ResponseWriter, status int, code string) {
	writePreviewJSON(w, status, map[string]string{"error": code})
}

func writePreviewJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
