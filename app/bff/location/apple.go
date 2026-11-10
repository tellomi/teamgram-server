package location

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/core/logx"
)

// appleAuth signs a MapKit JWT and exchanges it at /v1/token. The access
// token is cached until shortly before Apple's expiresInSeconds.
type appleAuth struct {
	teamID string
	keyID  string
	key    *ecdsa.PrivateKey
	base   string
	now    func() time.Time

	mu     sync.Mutex
	access string
	exp    time.Time
}

func newApple(opts Options, now func() time.Time) *appleAuth {
	team := strings.TrimSpace(opts.AppleTeamID)
	kid := strings.TrimSpace(opts.AppleKeyID)
	path := strings.TrimSpace(opts.AppleKeyPath)
	if team == "" && kid == "" && path == "" && len(opts.AppleKeyPEM) == 0 {
		return nil
	}
	pemBytes := opts.AppleKeyPEM
	if len(pemBytes) == 0 && path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			pemBytes = nil
		} else {
			pemBytes = raw
		}
	}
	key, err := parseECPrivateKey(pemBytes)
	if team == "" || kid == "" || err != nil || key == nil {
		logx.Errorf("venue apple provider disabled: TEAM_ID, KEY_ID, and a PKCS8 .p8 key are all required")
		return nil
	}
	return &appleAuth{teamID: team, keyID: kid, key: key, base: opts.AppleBase, now: now}
}

func parseECPrivateKey(raw []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	der := raw
	if block != nil {
		der = block.Bytes
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		ec, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("apple key is not EC")
		}
		return ec, nil
	}
	return x509.ParseECPrivateKey(der)
}

func (a *appleAuth) token(ctx context.Context, httpClient *http.Client) (string, error) {
	a.mu.Lock()
	if a.access != "" && a.now().Before(a.exp) {
		tok := a.access
		a.mu.Unlock()
		return tok, nil
	}
	a.mu.Unlock()
	jwt, err := mintDeveloperJWT(a.key, a.keyID, a.teamID, a.now())
	if err != nil {
		return "", err
	}
	reqCtx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, a.base+"/v1/token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("apple token")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("apple token http %d", resp.StatusCode)
	}
	var parsed struct {
		AccessToken      string `json:"accessToken"`
		ExpiresInSeconds int    `json:"expiresInSeconds"`
	}
	if json.Unmarshal(body, &parsed) != nil || parsed.AccessToken == "" {
		return "", fmt.Errorf("apple token empty")
	}
	ttl := time.Duration(parsed.ExpiresInSeconds) * time.Second
	if ttl < time.Minute {
		ttl = time.Minute
	}
	ttl -= 30 * time.Second
	a.mu.Lock()
	a.access = parsed.AccessToken
	a.exp = a.now().Add(ttl)
	a.mu.Unlock()
	return parsed.AccessToken, nil
}

func (a *appleAuth) clear() {
	a.mu.Lock()
	a.access = ""
	a.exp = time.Time{}
	a.mu.Unlock()
}

func mintDeveloperJWT(key *ecdsa.PrivateKey, kid, team string, now time.Time) (string, error) {
	header, err := json.Marshal(struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}{Alg: "ES256", Kid: kid, Typ: "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		Iss   string `json:"iss"`
		Iat   int64  `json:"iat"`
		Exp   int64  `json:"exp"`
		Scope string `json:"scope"`
	}{Iss: team, Iat: now.Unix(), Exp: now.Add(20 * time.Minute).Unix(), Scope: "server_api"})
	if err != nil {
		return "", err
	}
	signing := b64(header) + "." + b64(payload)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return "", err
	}
	sig := append(pad32(r), pad32(s)...)
	return signing + "." + b64(sig), nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func pad32(n *big.Int) []byte {
	out := make([]byte, 32)
	raw := n.Bytes()
	copy(out[32-len(raw):], raw)
	return out
}

func (c *Client) fetchApple(ctx context.Context, key string, lat, lng float64, query, offset string) ([]place, string, error) {
	if c.apple == nil {
		return nil, "", fmt.Errorf("apple off")
	}
	places, next, err := c.appleSearch(ctx, lat, lng, query, offset)
	if err != nil && strings.Contains(err.Error(), "http 401") {
		c.apple.clear()
		places, next, err = c.appleSearch(ctx, lat, lng, query, offset)
	}
	if err != nil {
		return nil, "", err
	}
	c.store(ctx, key, places, next)
	return places, next, nil
}

func (c *Client) appleSearch(ctx context.Context, lat, lng float64, query, offset string) ([]place, string, error) {
	tok, err := c.apple.token(ctx, c.http)
	if err != nil {
		return nil, "", err
	}
	q := url.Values{}
	loc := fmt.Sprintf("%.6f,%.6f", lat, lng)
	q.Set("searchLocation", loc)
	q.Set("lang", appleLang(ctx))
	path := "/v1/searchNearby"
	if strings.TrimSpace(query) != "" {
		path = "/v1/search"
		q.Set("q", strings.TrimSpace(query))
		q.Set("userLocation", loc)
		q.Set("resultTypeFilter", "Poi")
		q.Set("enablePagination", "true")
	}
	if offset != "" {
		q.Set("pageToken", offset)
	}
	started := time.Now()
	body, status, err := c.appleDo(ctx, path, q, tok)
	if err != nil {
		c.logUpstream(ctx, "apple", path, strconv.Itoa(status), "", started, err)
		return nil, "", err
	}
	var parsed appleResponse
	if json.Unmarshal(body, &parsed) != nil {
		c.logUpstream(ctx, "apple", path, strconv.Itoa(status), "", started, fmt.Errorf("apple json"))
		return nil, "", fmt.Errorf("apple json")
	}
	c.logUpstream(ctx, "apple", path, strconv.Itoa(status), "", started, nil)
	return applePlaces(parsed), parsed.next(), nil
}

func (c *Client) appleDo(ctx context.Context, path string, q url.Values, token string) ([]byte, int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.apple.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Tellomi/venue")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("apple %s", path)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode != http.StatusOK {
		return body, resp.StatusCode, fmt.Errorf("apple %s http %d", path, resp.StatusCode)
	}
	return body, resp.StatusCode, nil
}

func appleLang(ctx context.Context) string {
	if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
		code := strings.TrimSpace(md.GetLangCode())
		if code != "" && len(code) <= 32 && !strings.ContainsAny(code, " /?") {
			return code
		}
	}
	return "zh-Hans"
}

type appleResponse struct {
	Results    []applePlace `json:"results"`
	NextToken  string       `json:"nextPageToken"`
	Pagination struct {
		NextToken string `json:"nextPageToken"`
	} `json:"pagination"`
}

func (r appleResponse) next() string {
	if r.Pagination.NextToken != "" {
		return r.Pagination.NextToken
	}
	return r.NextToken
}

type applePlace struct {
	Name       string   `json:"name"`
	ID         string   `json:"id"`
	PlaceID    string   `json:"placeId"`
	Category   string   `json:"poiCategory"`
	Address    []string `json:"formattedAddressLines"`
	Coordinate struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"coordinate"`
}

func applePlaces(in appleResponse) []place {
	out := make([]place, 0, len(in.Results))
	seen := map[string]bool{}
	for _, p := range in.Results {
		name := strings.TrimSpace(p.Name)
		lat, lng := p.Coordinate.Latitude, p.Coordinate.Longitude
		if name == "" || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
			continue
		}
		id := strings.TrimSpace(p.ID)
		if id == "" {
			id = strings.TrimSpace(p.PlaceID)
		}
		if id == "" {
			id = fmt.Sprintf("apple:%.5f:%.5f:%s", lat, lng, truncate(name, 40))
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, place{
			ID:        id,
			Name:      name,
			Address:   strings.Join(p.Address, " "),
			VenueType: AppleVenueType(p.Category),
			Provider:  providerApple,
			Lat:       lat,
			Lng:       lng,
		})
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
