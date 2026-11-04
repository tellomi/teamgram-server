package chatbox

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
)

// Card is the Open Graph preview content-engine stores on a Discover post.
type Card struct {
	URL         string
	Title       string
	Description string
	Image       string
	Site        string
}

var (
	errPreviewDenied  = errors.New("preview denied")
	errPreviewTimeout = errors.New("preview timeout")
	errPreviewFetch   = errors.New("preview fetch")
)

// loadOpenGraph is replaced by tests. Production fetches the page.
var loadOpenGraph = fetchOpenGraph

// OpenGraph reads title, description, image URL, and site name for raw.
// Private, loopback, and link-local addresses are refused before the fetch,
// and again after DNS and redirects. The image URL is returned only when it
// passes the same checks. The fetch deadline is webTimeout.
func OpenGraph(ctx context.Context, raw string) (Card, error) {
	u, err := parseWebURL(raw)
	if err != nil {
		return Card{}, errPreviewDenied
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, webTimeout)
	defer cancel()
	res := loadOpenGraph(ctx, u)
	if res.blocked {
		return Card{}, errPreviewDenied
	}
	if res.failed {
		if res.pending || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Card{}, errPreviewTimeout
		}
		return Card{}, errPreviewFetch
	}
	meta := res.meta
	if meta.Title == "" {
		meta.Title = u.Host
	}
	if meta.Site == "" {
		meta.Site = u.Host
	}
	return Card{
		URL:         u.String(),
		Title:       meta.Title,
		Description: meta.Description,
		Image:       meta.ImageURL,
		Site:        meta.Site,
	}, nil
}

// filterImageURL is replaced by tests. Production drops private image URLs.
var filterImageURL = safeImageURL

func fetchOpenGraph(ctx context.Context, u *url.URL) fetchResult {
	body, ctype, final, err := httpGetContext(ctx, u, webHTMLLimit)
	if err != nil {
		if errors.Is(err, errBlockedTarget) {
			return fetchResult{failed: true, blocked: true}
		}
		pending := temporaryFetchErr(err) || errors.Is(err, context.DeadlineExceeded)
		return fetchResult{failed: true, pending: pending}
	}
	if !htmlType(ctype) {
		return fetchResult{failed: true}
	}
	meta := parseMeta(decodeHTML(body, ctype), pageBase(u, final))
	meta.ImageURL = filterImageURL(ctx, meta.ImageURL)
	return fetchResult{meta: meta}
}

// safeImageURL drops an image target that is not a public http(s) URL.
// A hostname is resolved and refused when any address is private, loopback,
// or link-local. A lookup failure drops the image and keeps the rest of the card.
func safeImageURL(ctx context.Context, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	u, err := parseWebURL(raw)
	if err != nil || u.User != nil {
		return ""
	}
	host := strings.Trim(hostOnly(u.Host), "[]")
	if host == "" || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		if blockedIP(ip) {
			return ""
		}
		return u.String()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return ""
	}
	for _, ip := range ips {
		if blockedIP(ip.IP) {
			return ""
		}
	}
	return u.String()
}
