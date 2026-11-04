package chatbox

import (
	"net"
	"net/url"
	"strings"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
)

// WebPage builds a webpage card for url.
//
// https://core.telegram.org/method/messages.getWebPage
// The card is the page's Open Graph / meta tags. Fetches refuse private,
// loopback, and link-local addresses, including after DNS and redirects.
// A hash match returns webPageNotModified. An empty or non-http URL returns
// WEBPAGE_CURL_FAILED. A page that cannot be loaded returns webPageEmpty,
// or webPagePending when the failure may succeed on retry.
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

// MediaForInputWebPage maps inputMediaWebPage to a client-decodable card.
//
// https://core.telegram.org/api/link-preview
// https://core.telegram.org/constructor/inputMediaWebPage
// Official clients send an https link with messages.sendMedia and
// inputMediaWebPage#c21b8849. The card is messageMediaWebPage#ddf10c3b
// wrapping webPage#e89c45b2, built from the fetched Open Graph tags.
// optional:flags.2 sends the text with no media when the URL cannot be
// described. Otherwise the error is WEBPAGE_CURL_FAILED.
// messageMediaUnsupported#9f84f49e is never returned: clients render that
// constructor as the "unsupported message" placeholder.
func MediaForInputWebPage(media *mtproto.InputMedia) (*mtproto.MessageMedia, error) {
	if media.GetPredicateName() != mtproto.Predicate_inputMediaWebPage {
		return nil, mtproto.ErrMediaInvalid
	}
	page, err := describe(media.GetUrl())
	if err != nil {
		if media.GetOptional() {
			return nil, nil
		}
		return nil, err
	}
	return mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{
		ForceLargeMedia: media.GetForceLargeMedia(),
		ForceSmallMedia: media.GetForceSmallMedia(),
		Webpage:         page,
	}).To_MessageMedia(), nil
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
	u, err := parseWebURL(raw)
	if err != nil {
		return nil, mtproto.ErrWebpageCurlFailed
	}
	if page, ok := cachedWebPage(u.String()); ok {
		return page, nil
	}
	page := fetchWebPage(u)
	storeWebPage(u.String(), page)
	return page, nil
}

func parseWebURL(raw string) (*url.URL, error) {
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
	if ip := net.ParseIP(hostOnly(u.Host)); ip != nil && blockedIP(ip) {
		return nil, mtproto.ErrWebpageCurlFailed
	}
	return u, nil
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
