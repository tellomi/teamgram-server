package chatbox

import (
	"context"
	"net/url"
	"strings"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
)

// InternalCard builds a page from data this server already has.
// handled means the URL is an internal link and must not be fetched,
// including when the page is webPageEmpty.
var InternalCard func(ctx context.Context, u *url.URL) (page *mtproto.WebPage, chats []*mtproto.Chat, users []*mtproto.User, handled bool)

// WebPage builds a webpage card for url.
//
// https://core.telegram.org/method/messages.getWebPage
// Internal tell.cc / t.me links are filled from local data. An external
// page is fetched only when TELLOMI_SERVER_LINK_FETCH=1. That fetch refuses
// private, loopback, and link-local addresses, including bracketed IPv6 and
// localhost, and again after DNS and redirects.
// A hash match returns webPageNotModified. An empty or non-http URL returns
// WEBPAGE_CURL_FAILED. A page that cannot be loaded returns webPageEmpty,
// or webPagePending when a fetch was attempted and may succeed on retry.
func WebPage(raw string, hash int32) (*mtproto.WebPage, error) {
	page, _, _, err := WebPageContext(context.Background(), raw, hash)
	return page, err
}

// WebPageContext is WebPage plus the chats and users a native card refers to.
func WebPageContext(ctx context.Context, raw string, hash int32) (*mtproto.WebPage, []*mtproto.Chat, []*mtproto.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	u, err := parseWebURL(raw)
	if err != nil {
		return nil, nil, nil, mtproto.ErrWebpageCurlFailed
	}
	page, chats, users, err := describeURL(ctx, u)
	if err != nil {
		return nil, nil, nil, err
	}
	if hash != 0 && page.GetPredicateName() == mtproto.Predicate_webPage && hash == page.GetHash() {
		return mtproto.MakeTLWebPageNotModified(&mtproto.WebPage{}).To_WebPage(), chats, users, nil
	}
	return page, chats, users, nil
}

// MediaForInputWebPage maps inputMediaWebPage to a client-decodable card.
//
// https://core.telegram.org/api/link-preview
// https://core.telegram.org/constructor/inputMediaWebPage
// Official clients send an https link with messages.sendMedia and
// inputMediaWebPage#c21b8849. The card is messageMediaWebPage#ddf10c3b
// wrapping webPage#e89c45b2. Internal links are filled locally. External
// pages are fetched only when TELLOMI_SERVER_LINK_FETCH=1.
// optional:flags.2 sends the text with no media when the URL cannot be
// parsed. Otherwise a bad URL is WEBPAGE_CURL_FAILED. A parsed URL with no
// card is messageMediaWebPage wrapping webPageEmpty, which clients hide.
// messageMediaUnsupported#9f84f49e is never returned: clients render that
// constructor as the "unsupported message" placeholder.
func MediaForInputWebPage(media *mtproto.InputMedia) (*mtproto.MessageMedia, error) {
	return MediaForInputWebPageContext(context.Background(), media)
}

// MediaForInputWebPageContext is MediaForInputWebPage with a request context.
func MediaForInputWebPageContext(ctx context.Context, media *mtproto.InputMedia) (*mtproto.MessageMedia, error) {
	if media.GetPredicateName() != mtproto.Predicate_inputMediaWebPage {
		return nil, mtproto.ErrMediaInvalid
	}
	page, _, _, err := WebPageContext(ctx, media.GetUrl(), 0)
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
// No link, and a link with no card, yield messageMediaEmpty.
// https://core.telegram.org/method/messages.getWebPagePreview
func WebPagePreview(text string, entities []*mtproto.MessageEntity) *mtproto.MessageMedia {
	media, _, _ := WebPagePreviewContext(context.Background(), text, entities)
	return media
}

// WebPagePreviewContext is WebPagePreview plus the chats and users on the card.
func WebPagePreviewContext(ctx context.Context, text string, entities []*mtproto.MessageEntity) (*mtproto.MessageMedia, []*mtproto.Chat, []*mtproto.User) {
	raw := FirstURL(text, entities)
	if raw == "" {
		return mtproto.MakeTLMessageMediaEmpty(&mtproto.MessageMedia{}).To_MessageMedia(), nil, nil
	}
	page, chats, users, err := WebPageContext(ctx, raw, 0)
	if err != nil || (page.GetPredicateName() != mtproto.Predicate_webPage && page.GetPredicateName() != mtproto.Predicate_webPagePending) {
		return mtproto.MakeTLMessageMediaEmpty(&mtproto.MessageMedia{}).To_MessageMedia(), nil, nil
	}
	return mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{
		Webpage: page,
	}).To_MessageMedia(), chats, users
}

func describeURL(ctx context.Context, u *url.URL) (*mtproto.WebPage, []*mtproto.Chat, []*mtproto.User, error) {
	if InternalCard != nil {
		page, chats, users, handled := InternalCard(ctx, u)
		if handled {
			if page == nil {
				page = emptyPage(u)
			}
			return page, chats, users, nil
		}
	}
	if page, ok := cachedWebPage(u.String()); ok {
		return page, nil, nil, nil
	}
	page := fetchWebPage(u)
	storeWebPage(u.String(), page)
	return page, nil, nil, nil
}

// AcceptURL parses an http(s) URL and rejects blocked hosts.
func AcceptURL(raw string) (*url.URL, error) {
	return parseWebURL(raw)
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
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || blockedHost(u.Host) {
		return nil, mtproto.ErrWebpageCurlFailed
	}
	return u, nil
}

// FirstURL is the first link in text or in a text-url entity.
func FirstURL(text string, entities []*mtproto.MessageEntity) string {
	return firstURL(text, entities)
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
