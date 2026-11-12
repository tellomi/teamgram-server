package chatbox

import (
	"context"
	"net/url"

	"github.com/teamgram/proto/mtproto"
)

// CachedExternalPage is a previously fetched external page for raw.
// ok is false when the URL has not been cached in this process (or Redis).
func CachedExternalPage(ctx context.Context, raw string) (*mtproto.WebPage, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	u, err := parseWebURL(raw)
	if err != nil {
		return nil, false
	}
	return pageCacheGet(ctx, pageCacheKey(u))
}

// PendingExternalPage is the webPagePending placeholder for raw. Its id
// matches the eventual webPage so a client can replace the card in place.
func PendingExternalPage(raw string) *mtproto.WebPage {
	u, err := parseWebURL(raw)
	if err != nil {
		return nil
	}
	return pendingPage(u)
}

// MediaFromPage wraps a fetched or pending page as messageMediaWebPage.
// webPageEmpty and other constructors yield nil, which means no card.
func MediaFromPage(page *mtproto.WebPage) *mtproto.MessageMedia {
	if page == nil {
		return nil
	}
	switch page.GetPredicateName() {
	case mtproto.Predicate_webPage, mtproto.Predicate_webPagePending:
		return mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{
			Webpage: page,
		}).To_MessageMedia()
	default:
		return nil
	}
}

// ParseExternalURL is parseWebURL for packages that attach a send card.
func ParseExternalURL(raw string) (*url.URL, error) {
	return parseWebURL(raw)
}

// InstallTestLoader replaces the HTTP fetcher with a fixed card. Tests
// must call the returned restore function.
func InstallTestLoader(title, site, description string) (restore func()) {
	prev := loadWebPage
	loadWebPage = func(context.Context, *url.URL) fetchResult {
		return fetchResult{meta: pageMeta{Title: title, Site: site, Description: description}}
	}
	return func() { loadWebPage = prev }
}
