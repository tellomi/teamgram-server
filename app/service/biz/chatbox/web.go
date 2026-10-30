package chatbox

import (
	"hash/fnv"
	"net/url"
	"strings"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// WebPage builds a webpage card for url.
//
// https://core.telegram.org/method/messages.getWebPage
// The card is derived from the URL itself. The server does not fetch the
// remote page (that would be an open SSRF). A hash match returns
// webPageNotModified. An empty or non-http URL returns WEBPAGE_CURL_FAILED,
// which is the documented failure for a page that cannot be loaded.
func WebPage(raw string, hash int32) (*mtproto.WebPage, error) {
	page, err := describe(raw)
	if err != nil {
		return nil, err
	}
	if hash != 0 && hash == page.Hash {
		return mtproto.MakeTLWebPageNotModified(&mtproto.WebPage{}).To_WebPage(), nil
	}
	return page, nil
}

// WebPagePreview returns the composer preview for the first link in text.
// No link yields messageMediaEmpty, which is what clients render as "no preview".
// https://core.telegram.org/method/messages.getWebPagePreview
func WebPagePreview(text string, entities []*mtproto.MessageEntity) *mtproto.MessageMedia {
	raw := firstURL(text, entities)
	if raw == "" {
		return mtproto.MakeTLMessageMediaEmpty(&mtproto.MessageMedia{}).To_MessageMedia()
	}
	page, err := describe(raw)
	if err != nil {
		return mtproto.MakeTLMessageMediaEmpty(&mtproto.MessageMedia{}).To_MessageMedia()
	}
	return mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{
		Webpage: page,
	}).To_MessageMedia()
}

func describe(raw string) (*mtproto.WebPage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, mtproto.ErrWebpageCurlFailed
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, mtproto.ErrWebpageCurlFailed
	}
	display := u.Host + u.RequestURI()
	if u.RequestURI() == "/" {
		display = u.Host
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(u.String()))
	sum := int32(h.Sum32() & 0x7fffffff)
	if sum == 0 {
		sum = 1
	}
	id := int64(sum)
	if id < 0 {
		id = -id
	}
	return mtproto.MakeTLWebPage(&mtproto.WebPage{
		Id:          id,
		Url_STRING:  u.String(),
		DisplayUrl:  display,
		Hash:        sum,
		Type:        wrapperspb.String("article"),
		SiteName:    wrapperspb.String(u.Host),
		Title:       wrapperspb.String(u.Host),
		Description: wrapperspb.String(u.String()),
	}).To_WebPage(), nil
}

func firstURL(text string, entities []*mtproto.MessageEntity) string {
	for _, e := range entities {
		if e == nil {
			continue
		}
		switch e.PredicateName {
		case mtproto.Predicate_messageEntityTextUrl:
			if e.Url != "" {
				return e.Url
			}
		case mtproto.Predicate_messageEntityUrl:
			if s := utf16Slice(text, e.Offset, e.Length); s != "" {
				return s
			}
		}
	}
	for _, field := range strings.Fields(text) {
		field = strings.Trim(field, "()[]<>\"'")
		if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
			return field
		}
	}
	return ""
}

func utf16Slice(s string, offset, length int32) string {
	if length <= 0 {
		return ""
	}
	units := utf16.Encode([]rune(s))
	if int(offset) >= len(units) {
		return ""
	}
	end := int(offset + length)
	if end > len(units) {
		end = len(units)
	}
	return string(utf16.Decode(units[offset:end]))
}
