package plugin

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type previewPlugin struct {
	page *mtproto.WebPage
}

func (previewPlugin) ReadReactionUnreadMessage(context.Context, int64, int32) error { return nil }
func (previewPlugin) UsernameResolveUsername(context.Context, *user.TLUserResolveUsername) (*mtproto.Peer, error) {
	return nil, nil
}
func (p previewPlugin) GetWebpagePreview(context.Context, string) (*mtproto.WebPage, error) {
	return p.page, nil
}

func TestRemakeKeepsClientAndInternalCards(t *testing.T) {
	ctx := context.Background()
	article := mtproto.MakeTLWebPage(&mtproto.WebPage{
		Type:  wrapperspb.String("article"),
		Title: wrapperspb.String("fetched"),
	}).To_WebPage()
	plug := previewPlugin{page: article}

	internal := mtproto.MakeTLMessage(&mtproto.Message{
		Message: "https://tell.cc/addstickers/erelpack",
		Media: mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{
			Webpage: mtproto.MakeTLWebPage(&mtproto.WebPage{
				Type:  wrapperspb.String("telegram_stickerset"),
				Title: wrapperspb.String("erel"),
			}).To_WebPage(),
		}).To_MessageMedia(),
	}).To_Message()
	out := RemakeMessage(ctx, plug, internal, 1, false, nil)
	if out.GetMedia().GetWebpage().GetTitle().GetValue() != "erel" {
		t.Fatalf("internal card replaced: %q", out.GetMedia().GetWebpage().GetTitle().GetValue())
	}

	client := mtproto.MakeTLMessage(&mtproto.Message{
		Message: "https://www.apple.com",
		Media: mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{
			Webpage: mtproto.MakeTLWebPage(&mtproto.WebPage{
				Type:   wrapperspb.String("article"),
				Title:  wrapperspb.String("iPhone"),
				Author: wrapperspb.String("unverified"),
			}).To_WebPage(),
		}).To_MessageMedia(),
	}).To_Message()
	out = RemakeMessage(ctx, plug, client, 1, false, nil)
	if out.GetMedia().GetWebpage().GetTitle().GetValue() != "iPhone" || out.GetMedia().GetWebpage().GetAuthor().GetValue() != "unverified" {
		t.Fatalf("client card replaced: %#v", out.GetMedia().GetWebpage())
	}

	out = RemakeMessage(ctx, plug, client, 1, true, nil)
	if out.GetMedia() != nil {
		t.Fatalf("no_webpage kept media %#v", out.GetMedia())
	}

	plain := mtproto.MakeTLMessage(&mtproto.Message{Message: "https://www.apple.com"}).To_Message()
	out = RemakeMessage(ctx, plug, plain, 1, false, nil)
	if out.GetMedia().GetWebpage().GetTitle().GetValue() != "fetched" {
		t.Fatalf("plain text was not filled: %#v", out.GetMedia())
	}
	emptyText := mtproto.MakeTLMessage(&mtproto.Message{Message: "https://www.apple.com/empty"}).To_Message()
	out = RemakeMessage(ctx, previewPlugin{page: mtproto.MakeTLWebPageEmpty(&mtproto.WebPage{}).To_WebPage()}, emptyText, 1, false, nil)
	if out.GetMedia() != nil {
		t.Fatalf("empty preview became media %#v", out.GetMedia())
	}
}
