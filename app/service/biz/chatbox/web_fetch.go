package chatbox

import (
	"bytes"
	"context"
	"hash/fnv"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chai2010/webp"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
	"github.com/teamgram/teamgram-server/pkg/dcoptions"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	webTimeout   = 8 * time.Second
	webHTMLLimit = 1 << 20
	webImgLimit  = 2 << 20
	webCacheTTL  = 10 * time.Minute
	webFailTTL   = time.Minute
	maxRedirects = 3
)

// pageMeta is the Open Graph / HTML meta card.
type pageMeta struct {
	Title       string
	Description string
	Site        string
	ImageURL    string
	Image       []byte
	ImageMIME   string
}

type fetchResult struct {
	meta    pageMeta
	pending bool
	failed  bool
}

// loadWebPage is replaced by tests. Production fetches the URL.
var loadWebPage = fetchRemote

type webCacheEntry struct {
	page  *mtproto.WebPage
	until time.Time
}

var webCache sync.Map

func cachedWebPage(key string) (*mtproto.WebPage, bool) {
	v, ok := webCache.Load(key)
	if !ok {
		return nil, false
	}
	ent := v.(webCacheEntry)
	if time.Now().After(ent.until) {
		webCache.Delete(key)
		return nil, false
	}
	return ent.page, true
}

func storeWebPage(key string, page *mtproto.WebPage) {
	ttl := webCacheTTL
	if page.GetPredicateName() != mtproto.Predicate_webPage {
		ttl = webFailTTL
	}
	webCache.Store(key, webCacheEntry{page: page, until: time.Now().Add(ttl)})
}

func fetchWebPage(u *url.URL) *mtproto.WebPage {
	res := loadWebPage(u)
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
	page := mtproto.MakeTLWebPage(&mtproto.WebPage{
		Id:          int64(sum),
		Url_STRING:  u.String(),
		DisplayUrl:  display,
		Hash:        sum,
		Type:        wrapperspb.String("article"),
		SiteName:    wrapperspb.String(meta.Site),
		Title:       wrapperspb.String(meta.Title),
		Description: wrapperspb.String(meta.Description),
	}).To_WebPage()
	if len(meta.Image) > 0 {
		if photo := storeWebPhoto(meta.Image, meta.ImageMIME); photo != nil {
			page.Photo = photo
		}
	}
	return page
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

func emptyPage(u *url.URL) *mtproto.WebPage {
	return mtproto.MakeTLWebPageEmpty(&mtproto.WebPage{
		Id:             int64(webHash(u.String(), "empty")),
		Url_STRING:     u.String(),
		Url_FLAGSTRING: wrapperspb.String(u.String()),
	}).To_WebPage()
}

func pendingPage(u *url.URL) *mtproto.WebPage {
	return mtproto.MakeTLWebPagePending(&mtproto.WebPage{
		Id:             int64(webHash(u.String(), "pending")),
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

func safeClient() *http.Client {
	dialer := &net.Dialer{Timeout: webTimeout}
	transport := &http.Transport{
		Proxy: nil, // never honor HTTP_PROXY; that is an SSRF bypass
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if ip := net.ParseIP(host); ip != nil {
				if blockedIP(ip) {
					return nil, mtproto.ErrWebpageCurlFailed
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			var last error
			for _, ip := range ips {
				if blockedIP(ip.IP) {
					last = mtproto.ErrWebpageCurlFailed
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
			if ip := net.ParseIP(hostOnly(req.URL.Host)); ip != nil && blockedIP(ip) {
				return mtproto.ErrWebpageCurlFailed
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return mtproto.ErrWebpageCurlFailed
			}
			return nil
		},
	}
}

func fetchRemote(u *url.URL) fetchResult {
	body, ctype, err := httpGet(u, webHTMLLimit)
	if err != nil {
		return fetchResult{failed: true, pending: temporaryFetchErr(err)}
	}
	if !htmlType(ctype) {
		return fetchResult{failed: true}
	}
	text := decodeHTML(body, ctype)
	meta := parseMeta(text, u)
	if meta.ImageURL != "" {
		if imgURL, err := u.Parse(meta.ImageURL); err == nil && (imgURL.Scheme == "http" || imgURL.Scheme == "https") {
			if ip := net.ParseIP(hostOnly(imgURL.Host)); ip == nil || !blockedIP(ip) {
				if raw, ictype, err := httpGet(imgURL, webImgLimit); err == nil && imageType(ictype, raw) {
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
	msg := err.Error()
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "Timeout") || strings.Contains(msg, "connection refused")
}

func httpGet(u *url.URL, limit int64) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "TellomiBot/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,image/*;q=0.8")
	resp, err := safeClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return nil, "", mtproto.ErrWebpageCurlFailed
	}
	if resp.StatusCode >= 400 {
		return nil, resp.Header.Get("Content-Type"), io.ErrUnexpectedEOF
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(raw)) > limit {
		raw = raw[:limit]
	}
	return raw, resp.Header.Get("Content-Type"), nil
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
	}
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

type storedPhoto struct {
	body []byte
	mime string
}

var webPhotos sync.Map

// WebPhoto is the og:image bytes for upload.getFile.
func WebPhoto(id int64) (body []byte, mime string, ok bool) {
	v, found := webPhotos.Load(id)
	if !found {
		return nil, "", false
	}
	p := v.(storedPhoto)
	return p.body, p.mime, len(p.body) > 0
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
	id := int64(webHash(string(body[:min(64, len(body))])))
	if id < 0 {
		id = -id
	}
	webPhotos.Store(id, storedPhoto{body: body, mime: "image/jpeg"})
	putWebPhotoDFS(id, body)
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
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err == nil {
		return img, format, nil
	}
	if img, err = webp.Decode(bytes.NewReader(raw)); err == nil {
		return img, "webp", nil
	}
	return nil, "", err
}

func init() {
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "\x89PNG", png.Decode, png.DecodeConfig)
	image.RegisterFormat("gif", "GIF8", gif.Decode, gif.DecodeConfig)
}

func putWebPhotoDFS(id int64, body []byte) {
	client, err := objectstore.Open()
	if err != nil || client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = client.Put(ctx, "photos", strconv.FormatInt(id, 10)+".dat", "image/jpeg", body)
}
