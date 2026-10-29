package linkpreview

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func mediaURL(cfg Config, filePath string, now time.Time) string {
	filePath = sanitizeMediaPath(filePath)
	if filePath == "" {
		return ""
	}
	if cfg.PublicMediaBaseURL != "" {
		return cfg.PublicMediaBaseURL + "/" + escapeMediaPath(filePath)
	}
	exp := now.Add(cfg.mediaTTL()).Unix()
	q := url.Values{}
	q.Set("p", filePath)
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", signMedia(cfg.mediaKey(), filePath, exp))
	rel := "/linkpreview/v1/media?" + q.Encode()
	if cfg.PublicURL != "" {
		return cfg.PublicURL + rel
	}
	return rel
}

func sanitizeMediaPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "/")
	if p == "" || len(p) > 512 || strings.Contains(p, "..") || strings.ContainsAny(p, "\\?\n\r") {
		return ""
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return p
}

func escapeMediaPath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func signMedia(key []byte, path string, exp int64) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(path))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyMedia(key []byte, path string, exp int64, sig string, now time.Time) bool {
	if path == "" || sig == "" {
		return false
	}
	path = sanitizeMediaPath(path)
	if path == "" {
		return false
	}
	skew := now.Unix()
	if exp < skew-60 || exp > skew+8*24*3600 {
		return false
	}
	want := signMedia(key, path, exp)
	got, err := hex.DecodeString(sig)
	wb, err2 := hex.DecodeString(want)
	if err != nil || err2 != nil || len(got) != len(wb) {
		return false
	}
	return hmac.Equal(got, wb)
}
