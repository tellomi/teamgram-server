package webpage

import (
	"context"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// PhotoLookup loads a photo the sender already uploaded.
// A nil photo means the id is not in this server's media store.
type PhotoLookup func(ctx context.Context, in *mtproto.InputPhoto) *mtproto.Photo

// ClientSupplied reports whether inputMediaWebPage carries the Tellomi
// extension (title, description, site name, or an uploaded photo).
func ClientSupplied(in *mtproto.InputMedia) bool {
	if in == nil || in.GetPredicateName() != mtproto.Predicate_inputMediaWebPage {
		return false
	}
	if strings.TrimSpace(in.GetTitle()) != "" || strings.TrimSpace(in.GetDescription()) != "" || strings.TrimSpace(in.GetProvider_STRING()) != "" {
		return true
	}
	photo := in.GetId_INPUTPHOTO()
	return photo != nil && photo.GetId() != 0
}

// Media is messages.sendMedia for inputMediaWebPage.
// A client-supplied card is stored as-is. Anything else follows the internal
// link path, then the optional server fetch.
func Media(ctx context.Context, in *mtproto.InputMedia, photo PhotoLookup) (*mtproto.MessageMedia, error) {
	if ClientSupplied(in) {
		return ClientMedia(ctx, in, photo)
	}
	return chatbox.MediaForInputWebPageContext(ctx, in)
}

// ClientMedia stores a card the sending client fetched on its own network.
func ClientMedia(ctx context.Context, in *mtproto.InputMedia, photo PhotoLookup) (*mtproto.MessageMedia, error) {
	page, err := clientPage(ctx, in, photo)
	if err != nil {
		if in.GetOptional() {
			return nil, nil
		}
		return nil, err
	}
	return mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{
		ForceLargeMedia: in.GetForceLargeMedia(),
		ForceSmallMedia: in.GetForceSmallMedia(),
		Webpage:         page,
	}).To_MessageMedia(), nil
}

func clientPage(ctx context.Context, in *mtproto.InputMedia, photo PhotoLookup) (*mtproto.WebPage, error) {
	if in == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	raw := strings.TrimSpace(in.GetUrl())
	if len(raw) > maxURLBytes || len(in.GetTitle()) > maxFieldBytes || len(in.GetDescription()) > maxFieldBytes || len(in.GetProvider_STRING()) > maxFieldBytes {
		return nil, mtproto.ErrMediaInvalid
	}
	u, err := chatbox.AcceptURL(raw)
	if err != nil {
		return nil, err
	}
	title := clamp(in.GetTitle(), maxTitleRunes)
	desc := clamp(in.GetDescription(), maxDescriptionRunes)
	site := clamp(in.GetProvider_STRING(), maxSiteRunes)
	if title == "" {
		title = u.Host
	}
	if site == "" {
		site = u.Hostname()
	}
	var pic *mtproto.Photo
	if src := in.GetId_INPUTPHOTO(); src != nil && src.GetId() != 0 && photo != nil {
		pic = photo(ctx, src)
	}
	typ := "article"
	if pic != nil {
		typ = "photo"
	}
	sum := hash32("client", u.String(), title, desc, site, typ)
	page := mtproto.MakeTLWebPage(&mtproto.WebPage{
		Id:             int64(sum),
		Url_STRING:     u.String(),
		Url_FLAGSTRING: wrapperspb.String(u.String()),
		DisplayUrl:     displayURL(u.String()),
		Hash:           sum,
		Type:           wrapperspb.String(typ),
		SiteName:       wrapperspb.String(site),
		Title:          wrapperspb.String(title),
		Author:         wrapperspb.String(UnverifiedAuthor),
		HasLargeMedia:  in.GetForceLargeMedia(),
		Photo:          pic,
		Date:           int32(time.Now().Unix()),
	}).To_WebPage()
	if desc != "" {
		page.Description = wrapperspb.String(desc)
	}
	return page, nil
}
