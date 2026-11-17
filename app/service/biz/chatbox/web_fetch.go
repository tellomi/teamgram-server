package chatbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chai2010/webp"
	dimaging "github.com/disintegration/imaging"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
	"github.com/teamgram/teamgram-server/app/service/mediablob"
	"github.com/teamgram/teamgram-server/pkg/dcoptions"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	webConnectTimeout = 3 * time.Second
	webTimeout        = 5 * time.Second
	webHTMLLimit      = 1 << 20
	webImgLimit       = 2 << 20
	webPositiveTTL    = 24 * time.Hour
	webNegativeTTL    = 20 * time.Minute
	maxRedirects      = 3
	webUserAgent      = "Mozilla/5.0 (compatible; TellomiBot/1.0; +https://tellomi.cn/bot)"

	// EnvServerLinkFetch opts in to the server downloading an external page.
	// Unset means messages.getWebPage does not open a connection to that site.
	// Internal tell.cc / tellomi.cn cards do not use this switch.
	EnvServerLinkFetch = "TELLOMI_SERVER_LINK_FETCH"

	// EnvLinkFetchProxy is an optional HTTP proxy used only by this fetcher.
	// SSRF checks run on the target before the request is handed to the proxy.
	// HTTP_PROXY and HTTPS_PROXY are ignored.
	EnvLinkFetchProxy = "TELLOMI_LINK_FETCH_PROXY"
)

// ServerFetchEnabled reports whether this process may download external pages
// for chat link cards. The default is off.
func ServerFetchEnabled() bool {
	return os.Getenv(EnvServerLinkFetch) == "1"
}

// pageMeta is the Open Graph / HTML meta card.
type pageMeta struct {
	Title       string
	Description string
	Site        string
	ImageURL    string
	Image       []byte
	ImageMIME   string
	OGType      string
	HasVideo    bool
	VideoW      int
	VideoH      int
}

type fetchResult struct {
	meta    pageMeta
	pending bool
	failed  bool
	// blocked is a private, loopback, or link-local target. Chat previews
	// still become an empty page. The Discover card returns 400.
	blocked bool
}

// errBlockedTarget is returned when dial or a redirect hits an address
// blockedHost would refuse. It is wrapped with WEBPAGE_CURL_FAILED.
var errBlockedTarget = errors.New("blocked target")

func blockedTargetErr() error {
	return fmt.Errorf("%w: %w", mtproto.ErrWebpageCurlFailed, errBlockedTarget)
}

// webHTTPClient is replaced by tests. Production uses safeClient.
var webHTTPClient = safeClient

// vetRequest is replaced by tests that fetch a local server on a random port.
var vetRequest = vetTarget

// webResolver performs DNS for SSRF checks and the safe HTTP dialer.
// Tests replace it with a Resolver whose Dial points at a hanging listener.
var webResolver = net.DefaultResolver

func webLookupIP(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	names, err := webResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]net.IPAddr, 0, len(names))
	for _, name := range names {
		ip := net.ParseIP(name)
		if ip == nil {
			continue
		}
		out = append(out, net.IPAddr{IP: ip})
	}
	if len(out) == 0 {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return out, nil
}

// loadWebPage is replaced by tests. Production fetches the URL and must
// honor the request context so DNS and HTTP cancel at the deadline.
var loadWebPage = fetchRemote

func fetchWebPage(ctx context.Context, u *url.URL) *mtproto.WebPage {
	if ctx == nil {
		ctx = context.Background()
	}
	if !ServerFetchEnabled() {
		return emptyPage(u)
	}
	key := pageCacheKey(u)
	if page, ok := pageCacheGet(ctx, key); ok {
		return page
	}
	v, err, _ := webFlight.Do(key, func() (any, error) {
		if page, ok := pageCacheGet(ctx, key); ok {
			return page, nil
		}
		release, allowed := acquireFetch(u.Hostname())
		if !allowed {
			return emptyPage(u), nil
		}
		defer release()
		res := loadWebPage(ctx, u)
		page := pageFromFetch(u, res)
		ttl := webPositiveTTL
		if page.GetPredicateName() != mtproto.Predicate_webPage {
			ttl = webNegativeTTL
		}
		pageCacheSet(ctx, key, page, ttl)
		return page, nil
	})
	if err != nil || v == nil {
		return emptyPage(u)
	}
	page, _ := v.(*mtproto.WebPage)
	if page == nil {
		return emptyPage(u)
	}
	return page
}

func pageFromFetch(u *url.URL, res fetchResult) *mtproto.WebPage {
	if res.failed {
		if res.pending {
			return pendingPage(u)
		}
		return emptyPage(u)
	}
	meta := res.meta
	if meta.Title == "" {
		meta.Title = u.Host
	}
	if meta.Site == "" {
		meta.Site = u.Host
	}
	display := u.Host + u.RequestURI()
	if u.RequestURI() == "/" {
		display = u.Host
	}
	sum := webHash(u.String(), meta.Title, meta.Description)
	var iw, ih int
	var photo *mtproto.Photo
	if len(meta.Image) > 0 {
		photo = storeWebPhoto(meta.Image, meta.ImageMIME)
		if photo != nil && len(photo.GetSizes()) > 0 {
			iw = int(photo.GetSizes()[0].GetW())
			ih = int(photo.GetSizes()[0].GetH())
			// A square og:image (Douyin covers are often 1:1) must not
			// become the bubble when the video itself is tall or wide.
			// The pixels stay as fetched; only the stored size changes.
			if iw > 0 && iw == ih && meta.VideoW > 0 && meta.VideoH > 0 && meta.VideoW != meta.VideoH {
				iw, ih = fitPreview(meta.VideoW, meta.VideoH)
				photo.GetSizes()[0].W = int32(iw)
				photo.GetSizes()[0].H = int32(ih)
			}
		}
	}
	page := mtproto.MakeTLWebPage(&mtproto.WebPage{
		Id:          pageID(u),
		Url_STRING:  u.String(),
		DisplayUrl:  display,
		Hash:        sum,
		Type:        wrapperspb.String(previewType(meta, iw, ih)),
		SiteName:    wrapperspb.String(meta.Site),
		Title:       wrapperspb.String(meta.Title),
		Description: wrapperspb.String(meta.Description),
		Photo:       photo,
	}).To_WebPage()
	return page
}

// previewType is the webPage.type clients use to pick a layout.
// "article" is drawn in a square slot, which center-crops a tall cover.
// A video page, or any non-square image, keeps its own aspect.
func previewType(meta pageMeta, w, h int) string {
	og := strings.ToLower(meta.OGType)
	if strings.Contains(og, "video") || meta.HasVideo || (meta.VideoW > 0 && meta.VideoH > 0) {
		return "video"
	}
	if w > 0 && h > 0 && w != h {
		return "photo"
	}
	return "article"
}

func fitPreview(w, h int) (int, int) {
	const longSide = 320
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if w >= h {
		if w <= longSide {
			return w, h
		}
		nh := h * longSide / w
		if nh < 1 {
			nh = 1
		}
		return longSide, nh
	}
	if h <= longSide {
		return w, h
	}
	nw := w * longSide / h
	if nw < 1 {
		nw = 1
	}
	return nw, longSide
}

func webHash(parts ...string) int32 {
	h := fnv.New32a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	sum := int32(h.Sum32() & 0x7fffffff)
	if sum == 0 {
		return 1
	}
	return sum
}

func pageID(u *url.URL) int64 {
	if u == nil {
		return 1
	}
	id := int64(webHash(u.String(), "id"))
	if id == 0 {
		return 1
	}
	return id
}

func emptyPage(u *url.URL) *mtproto.WebPage {
	return mtproto.MakeTLWebPageEmpty(&mtproto.WebPage{
		Id:             pageID(u),
		Url_STRING:     u.String(),
		Url_FLAGSTRING: wrapperspb.String(u.String()),
	}).To_WebPage()
}

func pendingPage(u *url.URL) *mtproto.WebPage {
	return mtproto.MakeTLWebPagePending(&mtproto.WebPage{
		Id:             pageID(u),
		Url_STRING:     u.String(),
		Url_FLAGSTRING: wrapperspb.String(u.String()),
		Date:           int32(time.Now().Add(time.Minute).Unix()),
	}).To_WebPage()
}

func blockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	ip = ip.To16()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	// Carrier-grade NAT. IsPrivate does not include it.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64 {
		return true
	}
	return false
}

func hostOnly(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}

// blockedHost is the check that runs before a request. Bracketed IPv6 is
// an address: [::1] is loopback even when the port is omitted. localhost
// is refused here so it is not looked up. DNS answers are checked again
// in the dialer.
func blockedHost(hostport string) bool {
	host := strings.Trim(hostOnly(hostport), "[]")
	if host == "" || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && blockedIP(ip)
}

func safeClient() *http.Client {
	proxyURL := linkFetchProxy()
	dialer := &net.Dialer{Timeout: webConnectTimeout}
	transport := &http.Transport{
		// nil ignores HTTP_PROXY. A configured proxy is explicit and is not
		// taken from the environment.
		Proxy: httpProxy(proxyURL),
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if proxyURL != nil {
				// The dial is to the proxy. The target was checked first.
				return dialer.DialContext(ctx, network, addr)
			}
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if port != "80" && port != "443" {
				return nil, blockedTargetErr()
			}
			if ip := net.ParseIP(host); ip != nil {
				if blockedIP(ip) {
					return nil, blockedTargetErr()
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			}
			ips, err := webLookupIP(ctx, host)
			if err != nil {
				return nil, err
			}
			var last error
			for _, ip := range ips {
				if blockedIP(ip.IP) {
					last = blockedTargetErr()
					continue
				}
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			if last == nil {
				last = mtproto.ErrWebpageCurlFailed
			}
			return nil, last
		},
	}
	return &http.Client{
		Timeout:   webTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return mtproto.ErrWebpageCurlFailed
			}
			if err := vetRequest(req.Context(), req.URL); err != nil {
				return err
			}
			return nil
		},
	}
}

func httpProxy(proxyURL *url.URL) func(*http.Request) (*url.URL, error) {
	if proxyURL == nil {
		return nil
	}
	return func(*http.Request) (*url.URL, error) {
		return proxyURL, nil
	}
}

func linkFetchProxy() *url.URL {
	raw := strings.TrimSpace(os.Getenv(EnvLinkFetchProxy))
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil
	}
	return u
}

// vetTarget rejects a URL that must not be fetched: a non-80/443 port, a
// private address, or a name that resolves to one. Callers run this before
// a proxy sees the request. A DNS failure is returned as-is so the caller
// can treat it as a fetch error rather than an SSRF block.
func vetTarget(ctx context.Context, u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return blockedTargetErr()
	}
	if !allowedWebPort(u) || blockedHost(u.Host) {
		return blockedTargetErr()
	}
	host := strings.Trim(hostOnly(u.Host), "[]")
	if ip := net.ParseIP(host); ip != nil {
		if blockedIP(ip) {
			return blockedTargetErr()
		}
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ips, err := webLookupIP(ctx, host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return mtproto.ErrWebpageCurlFailed
	}
	for _, ip := range ips {
		if blockedIP(ip.IP) {
			return blockedTargetErr()
		}
	}
	return nil
}

func allowedWebPort(u *url.URL) bool {
	if u == nil {
		return false
	}
	port := u.Port()
	switch u.Scheme {
	case "https":
		return port == "" || port == "443"
	case "http":
		return port == "" || port == "80"
	default:
		return false
	}
}

func fetchRemote(ctx context.Context, u *url.URL) fetchResult {
	if ctx == nil {
		ctx = context.Background()
	}
	body, ctype, final, err := httpGetContext(ctx, u, webHTMLLimit)
	if err != nil {
		return fetchResult{failed: true, pending: temporaryFetchErr(err) && !errors.Is(err, errBlockedTarget), blocked: errors.Is(err, errBlockedTarget)}
	}
	if !htmlType(ctype) {
		return fetchResult{failed: true}
	}
	text := decodeHTML(body, ctype)
	meta := parseMeta(text, pageBase(u, final))
	if meta.ImageURL != "" {
		if imgURL, err := u.Parse(meta.ImageURL); err == nil && (imgURL.Scheme == "http" || imgURL.Scheme == "https") {
			if ip := net.ParseIP(hostOnly(imgURL.Host)); ip == nil || !blockedIP(ip) {
				if raw, ictype, _, err := httpGetContext(ctx, imgURL, webImgLimit); err == nil && imageType(ictype, raw) {
					meta.Image = raw
					meta.ImageMIME = ictype
				}
			}
		}
	}
	return fetchResult{meta: meta}
}

func temporaryFetchErr(err error) bool {
	if err == nil {
		return false
	}
	// The request context deadline (pendingWebpageTimeout) is the fetch budget.
	// When it fires, return a finished failure (webPageEmpty), not webPagePending.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "Timeout") || strings.Contains(msg, "connection refused")
}

func httpGet(u *url.URL, limit int64) ([]byte, string, error) {
	body, ctype, _, err := httpGetContext(context.Background(), u, limit)
	return body, ctype, err
}

func httpGetContext(ctx context.Context, u *url.URL, limit int64) ([]byte, string, *url.URL, error) {
	if err := vetRequest(ctx, u); err != nil {
		return nil, "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", nil, err
	}
	req.Header.Set("User-Agent", webUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,image/*;q=0.8")
	resp, err := webHTTPClient().Do(req)
	if err != nil {
		return nil, "", nil, err
	}
	defer resp.Body.Close()
	final := pageBase(u, nil)
	if resp.Request != nil {
		final = pageBase(u, resp.Request.URL)
	}
	if resp.StatusCode >= 500 {
		return nil, "", final, mtproto.ErrWebpageCurlFailed
	}
	if resp.StatusCode >= 400 {
		return nil, resp.Header.Get("Content-Type"), final, io.ErrUnexpectedEOF
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", final, err
	}
	if int64(len(raw)) > limit {
		raw = raw[:limit]
	}
	return raw, resp.Header.Get("Content-Type"), final, nil
}

// pageBase is the URL relative image paths resolve against. Redirects keep
// the final page, so og:image "/pic.jpg" stays on the site that sent the HTML.
func pageBase(original, final *url.URL) *url.URL {
	if final != nil && final.Host != "" && (final.Scheme == "http" || final.Scheme == "https") {
		return final
	}
	return original
}

func htmlType(ctype string) bool {
	ctype = strings.ToLower(ctype)
	return strings.Contains(ctype, "text/html") || strings.Contains(ctype, "application/xhtml") || ctype == ""
}

func imageType(ctype string, raw []byte) bool {
	ctype = strings.ToLower(ctype)
	if strings.Contains(ctype, "image/jpeg") || strings.Contains(ctype, "image/png") || strings.Contains(ctype, "image/gif") || strings.Contains(ctype, "image/webp") {
		return true
	}
	if len(raw) >= 3 && raw[0] == 0xff && raw[1] == 0xd8 {
		return true
	}
	if len(raw) >= 8 && bytes.HasPrefix(raw, []byte("\x89PNG")) {
		return true
	}
	if len(raw) >= 12 && bytes.Contains(raw[:12], []byte("WEBP")) {
		return true
	}
	return false
}

func decodeHTML(raw []byte, ctype string) string {
	ctype = strings.ToLower(ctype)
	switch {
	case strings.Contains(ctype, "gbk"), strings.Contains(ctype, "gb2312"), strings.Contains(ctype, "gb18030"):
		out, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), raw)
		if err == nil && utf8.Valid(out) {
			return string(out)
		}
	}
	if !utf8.Valid(raw) {
		if out, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), raw); err == nil && utf8.Valid(out) {
			return string(out)
		}
	}
	reader, err := charset.NewReader(bytes.NewReader(raw), ctype)
	if err != nil {
		return string(raw)
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, webHTMLLimit))
	if err != nil || len(decoded) == 0 {
		return string(raw)
	}
	return string(decoded)
}

func parseMeta(text string, base *url.URL) pageMeta {
	var meta pageMeta
	z := html.NewTokenizer(strings.NewReader(text))
	var title strings.Builder
	inTitle := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		tok := z.Token()
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			switch tok.Data {
			case "title":
				inTitle = true
				title.Reset()
			case "meta":
				applyMeta(&meta, tok.Attr)
			}
		case html.TextToken:
			if inTitle {
				title.WriteString(tok.Data)
			}
		case html.EndTagToken:
			if tok.Data == "title" {
				inTitle = false
			}
		}
	}
	if meta.Title == "" {
		meta.Title = strings.TrimSpace(title.String())
	}
	meta.Title = oneLine(meta.Title)
	meta.Description = oneLine(meta.Description)
	meta.Site = oneLine(meta.Site)
	if meta.ImageURL != "" && base != nil {
		if ref, err := base.Parse(meta.ImageURL); err == nil {
			meta.ImageURL = ref.String()
		}
	}
	return meta
}

func applyMeta(meta *pageMeta, attrs []html.Attribute) {
	var key, content string
	for _, a := range attrs {
		switch strings.ToLower(a.Key) {
		case "property", "name", "itemprop":
			if key == "" {
				key = strings.ToLower(a.Val)
			}
		case "content":
			content = a.Val
		case "charset":
			// charset is handled from Content-Type and the body
		}
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	switch key {
	case "og:title", "twitter:title":
		if meta.Title == "" {
			meta.Title = content
		}
	case "og:description", "twitter:description", "description":
		if meta.Description == "" {
			meta.Description = content
		}
	case "og:site_name":
		if meta.Site == "" {
			meta.Site = content
		}
	case "og:image", "og:image:url", "twitter:image":
		if meta.ImageURL == "" {
			meta.ImageURL = content
		}
	case "og:type":
		if meta.OGType == "" {
			meta.OGType = content
		}
	case "og:video", "og:video:url":
		meta.HasVideo = true
	case "og:video:width":
		if meta.VideoW == 0 {
			meta.VideoW = metaInt(content)
		}
	case "og:video:height":
		if meta.VideoH == 0 {
			meta.VideoH = metaInt(content)
		}
	}
}

func metaInt(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= 300 {
		return s
	}
	s = s[:300]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

type storedPhoto struct {
	body    []byte
	mime    string
	path    string
	missing bool
}

var webPhotos sync.Map

// readWebObject loads one persisted link-preview object. Tests replace it.
// Production reads object storage.
var readWebObject = readWebPhotoObject

// SetWebPhotoReaderForTest installs fn as the persisted link-preview read.
// nil restores the production reader.
func SetWebPhotoReaderForTest(fn func(path string) ([]byte, error)) {
	if fn == nil {
		readWebObject = readWebPhotoObject
		return
	}
	readWebObject = fn
}

// WebPhoto is the og:image kept in this process. It does not touch object
// storage: a miss is not a reason to dial.
func WebPhoto(id int64) (body []byte, mime string, ok bool) {
	body, mime, ok, _ = webPhotoMem(id)
	return body, mime, ok
}

// WebPhotoForDownload is upload.getFile for a photo location.
//
// Bytes already stored in this process are returned for any id. A persisted
// object is read only when accessHash == id, which is the hash storeWebPhoto
// issues for a link preview. An ordinary photo uses a different hash. Probing
// linkpreview/<id>.dat for that id calls objectstore.Open, and Open builds a
// new S3 client (bucket probe plus connection warmup) on every call. That
// probe is what made upload.getFile wait about 7s before dfs, including for
// a thumbnail that was already on the hot tier.
func WebPhotoForDownload(id, accessHash int64) (body []byte, mime string, ok bool) {
	body, mime, ok, known := webPhotoMem(id)
	if ok || known {
		return body, mime, ok
	}
	if id == 0 || accessHash != id {
		return nil, "", false
	}
	return loadPersistedWebPhoto(id)
}

func webPhotoMem(id int64) (body []byte, mime string, ok, known bool) {
	v, found := webPhotos.Load(id)
	if !found {
		return nil, "", false, false
	}
	p := v.(storedPhoto)
	if p.missing || len(p.body) == 0 {
		return nil, "", false, true
	}
	return p.body, p.mime, true, true
}

func loadPersistedWebPhoto(id int64) ([]byte, string, bool) {
	path := webPhotoPath(id)
	raw, err := readWebObject(path)
	if err != nil || len(raw) == 0 {
		// A definitive miss stays missing. A timeout or a client that is
		// still starting can succeed on a later download, so it is not cached.
		if err == nil || errors.Is(err, objectstore.ErrNotFound) {
			webPhotos.Store(id, storedPhoto{missing: true})
		}
		return nil, "", false
	}
	webPhotos.Store(id, storedPhoto{body: raw, mime: "image/jpeg", path: path})
	return raw, "image/jpeg", true
}

var (
	persistedWebMu  sync.Mutex
	persistedWebCli *objectstore.Client
)

func readWebPhotoObject(path string) ([]byte, error) {
	// Open builds a new client and warms connections. One client per process
	// is enough; a download must not pay that again.
	persistedWebMu.Lock()
	client := persistedWebCli
	if client == nil {
		opened, err := objectstore.Open()
		if err != nil || opened == nil {
			persistedWebMu.Unlock()
			if err == nil {
				err = errors.New("object storage is not configured")
			}
			return nil, err
		}
		persistedWebCli = opened
		client = opened
	}
	persistedWebMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), webTimeout)
	defer cancel()
	return client.GetPhoto(ctx, path)
}

func storeWebPhoto(raw []byte, mime string) *mtproto.Photo {
	img, format, err := decodePreview(raw)
	if err != nil || img == nil {
		return nil
	}
	var jpg bytes.Buffer
	if err = jpeg.Encode(&jpg, img, &jpeg.Options{Quality: 75}); err != nil {
		return nil
	}
	body := jpg.Bytes()
	id := webPhotoID(body)
	path := persistWebPhoto(id, body)
	webPhotos.Store(id, storedPhoto{body: body, mime: "image/jpeg", path: path})
	b := img.Bounds()
	_ = format
	_ = mime
	return mtproto.MakeTLPhoto(&mtproto.Photo{
		Id:         id,
		AccessHash: id,
		Date:       int32(time.Now().Unix()),
		DcId:       dcoptions.FileDC(),
		Sizes: []*mtproto.PhotoSize{
			mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{
				Type:  mtproto.PhotoSZMediumType,
				W:     int32(b.Dx()),
				H:     int32(b.Dy()),
				Size2: int32(len(body)),
			}).To_PhotoSize(),
		},
	}).To_Photo()
}

func decodePreview(raw []byte) (image.Image, string, error) {
	// AutoOrientation bakes EXIF into the pixels. A phone JPEG is often
	// stored landscape with a rotate-90 tag; ignoring the tag stores a
	// landscape box and the client center-crops the upright picture.
	if img, err := dimaging.Decode(bytes.NewReader(raw), dimaging.AutoOrientation(true)); err == nil {
		return img, "image", nil
	}
	if img, err := webp.Decode(bytes.NewReader(raw)); err == nil {
		return img, "webp", nil
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", err
	}
	return img, format, nil
}

func init() {
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "\x89PNG", png.Decode, png.DecodeConfig)
	image.RegisterFormat("gif", "GIF8", gif.Decode, gif.DecodeConfig)
}

func webPhotoID(body []byte) int64 {
	sum := sha256.Sum256(body)
	id := int64(binary.BigEndian.Uint64(sum[:8]) & 0x7fffffffffffffff)
	if id == 0 {
		return 1
	}
	return id
}

// writeWebObject and claimWebBlob are replaced by tests. Production writes
// the JPEG through R2 dedup (ADR-0082) when MySQL and object storage are set.
var writeWebObject = writeWebPhotoObject
var claimWebBlob = claimWebPhotoBlob

func persistWebPhoto(id int64, body []byte) string {
	sum := sha256.Sum256(body)
	path := webPhotoPath(id)
	if got, reused := claimWebBlob(hex.EncodeToString(sum[:]), int64(len(body)), path); got != "" {
		path = got
		if reused {
			return path
		}
	}
	_ = writeWebObject(path, body)
	return path
}

func claimWebPhotoBlob(sha string, size int64, proposed string) (string, bool) {
	store := webBlobStore()
	if store == nil {
		return proposed, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	claim, err := store.Claim(ctx, mediablob.ClassPhotos, mediablob.KindPhoto, photoIDFromPath(proposed), sha, size, proposed)
	if err != nil || claim.Path == "" {
		return proposed, false
	}
	return claim.Path, claim.Reused
}

func webPhotoPath(id int64) string {
	return fmt.Sprintf("linkpreview/%d.dat", id)
}

func photoIDFromPath(path string) int64 {
	base := strings.TrimSuffix(path, ".dat")
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	id, err := strconv.ParseInt(base, 10, 64)
	if err != nil || id == 0 {
		return 1
	}
	return id
}

func writeWebPhotoObject(path string, body []byte) error {
	client, err := objectstore.Open()
	if err != nil || client == nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return client.PutPhoto(ctx, path, "image/jpeg", body)
}
