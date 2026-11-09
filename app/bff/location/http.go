package location

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

const upstreamTimeout = 2 * time.Second

// redact strips API keys from a string before it is logged.
func redact(s, key, sig, tencent string) string {
	for _, secret := range []string{key, sig, tencent} {
		if secret == "" {
			continue
		}
		s = strings.ReplaceAll(s, secret, "redacted")
	}
	return s
}

func (c *Client) logUpstream(ctx context.Context, provider, path, status, infocode string, started time.Time, err error) {
	ms := time.Since(started).Milliseconds()
	if err != nil {
		logx.WithContext(ctx).Errorf("venue upstream provider=%s path=%s ms=%d err=%s", provider, path, ms, redact(err.Error(), c.amapKey, c.amapSig, c.tencentKey))
		return
	}
	logx.WithContext(ctx).Infof("venue upstream provider=%s path=%s status=%s infocode=%s ms=%d", provider, path, status, infocode, ms)
}

func (c *Client) get(ctx context.Context, provider, raw string, signed bool) (body []byte, status int, err error) {
	if signed && c.amapSig != "" {
		raw, err = signAMap(raw, c.amapSig)
		if err != nil {
			return nil, 0, err
		}
	}
	reqCtx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Tellomi/venue")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode != http.StatusOK {
		return body, resp.StatusCode, fmt.Errorf("http %d", resp.StatusCode)
	}
	return body, resp.StatusCode, nil
}

// signAMap appends the 数字签名 parameter. The secret is not a query field.
// https://lbs.amap.com/api/webservice/guide/tools/signature
func signAMap(raw, secret string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		if k == "sig" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(q.Get(k))
	}
	b.WriteString(secret)
	sum := md5.Sum([]byte(b.String()))
	q.Set("sig", hex.EncodeToString(sum[:]))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" || string(b) == "[]" || string(b) == `""` {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return nil
		}
		*f = flexString(s)
	}
	return nil
}

func (f flexString) String() string { return strings.TrimSpace(string(f)) }

func joinAddr(parts ...string) string {
	var b strings.Builder
	last := ""
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == last {
			continue
		}
		if strings.Contains(b.String(), part) {
			continue
		}
		b.WriteString(part)
		last = part
	}
	return b.String()
}

func isPNG(b []byte) bool {
	return len(b) > 8 && b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G'
}

func quotaBody(body []byte) bool {
	var parsed struct {
		Infocode string `json:"infocode"`
		Status   int    `json:"status"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return false
	}
	return amapQuota(parsed.Infocode) || tencentQuota(parsed.Status)
}
