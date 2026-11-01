package e2e

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	messages "github.com/teamgram/teamgram-server/app/bff/messages"
	"github.com/teamgram/teamgram-server/app/bff/messages/p1rest"
)

// Settings opens account.getChatThemes and account.getWallPapers before the
// user picks a chat theme or a solid background.
func TestChatThemeAndWallpaperCatalog(t *testing.T) {
	ctx := context.Background()
	p1rest.Reset()

	appThemes, err := messages.NewThemes().AccountGetThemes(ctx, &mtproto.TLAccountGetThemes{Format: "android"})
	if err != nil || len(appThemes.GetThemes()) < 2 || appThemes.GetThemes()[0].GetSlug() == "" {
		t.Fatalf("app themes: %v %#v", err, appThemes)
	}
	if err := appThemes.Encode(mtproto.NewEncodeBuf(4096), 232); err != nil {
		t.Fatal(err)
	}
	if _, err := messages.NewThemes().AccountGetThemes(ctx, &mtproto.TLAccountGetThemes{Format: "desktop"}); err == nil {
		t.Fatal("desktop format was accepted")
	}

	themes, err := messages.NewThemes().AccountGetChatThemes(ctx, &mtproto.TLAccountGetChatThemes{})
	if err != nil {
		t.Fatal(err)
	}
	if themes.GetPredicateName() != mtproto.Predicate_account_themes || len(themes.GetThemes()) < 4 || themes.GetHash() == 0 {
		t.Fatalf("themes: %s len %d", themes.GetPredicateName(), len(themes.GetThemes()))
	}
	if err := themes.Encode(mtproto.NewEncodeBuf(8192), 232); err != nil {
		t.Fatal(err)
	}
	cached, err := messages.NewThemes().AccountGetChatThemes(ctx, &mtproto.TLAccountGetChatThemes{Hash: themes.GetHash()})
	if err != nil || cached.GetPredicateName() != mtproto.Predicate_account_themesNotModified {
		t.Fatalf("themes not modified: %v %s", err, cached.GetPredicateName())
	}

	picked := themes.GetThemes()[0]
	if picked.GetEmoticon().GetValue() == "" || len(picked.GetSettings()) != 2 {
		t.Fatalf("picked theme %#v", picked)
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7, AccessHash: 1}).To_InputPeer()
	up, err := p1rest.SetChatTheme(4, &mtproto.TLMessagesSetChatTheme{
		Peer:  peer,
		Theme: mtproto.MakeTLInputChatTheme(&mtproto.InputChatTheme{Emoticon: picked.GetEmoticon().GetValue()}).To_InputChatTheme(),
	})
	if err != nil || up.GetPredicateName() != mtproto.Predicate_updates {
		t.Fatalf("set chat theme: %v %#v", err, up)
	}

	walls, err := messages.NewWallpapers().AccountGetWallPapers(ctx, &mtproto.TLAccountGetWallPapers{})
	if err != nil {
		t.Fatal(err)
	}
	if walls.GetPredicateName() != mtproto.Predicate_account_wallPapers || len(walls.GetWallpapers()) < 4 || walls.GetHash() == 0 {
		t.Fatalf("walls: %s len %d", walls.GetPredicateName(), len(walls.GetWallpapers()))
	}
	if err := walls.Encode(mtproto.NewEncodeBuf(8192), 232); err != nil {
		t.Fatal(err)
	}
	again, err := messages.NewWallpapers().AccountGetWallPapers(ctx, &mtproto.TLAccountGetWallPapers{Hash: walls.GetHash()})
	if err != nil || again.GetPredicateName() != mtproto.Predicate_account_wallPapersNotModified {
		t.Fatalf("walls not modified: %v %s", err, again.GetPredicateName())
	}

	var solid *mtproto.WallPaper
	for _, paper := range walls.GetWallpapers() {
		if paper.GetPredicateName() == mtproto.Predicate_wallPaperNoFile && paper.GetSettings() != nil && paper.GetSettings().GetBackgroundColor() != nil {
			solid = paper
			break
		}
	}
	if solid == nil {
		t.Fatal("no solid wallpaper")
	}
	got, err := messages.NewWallpapers().AccountGetWallPaper(ctx, &mtproto.TLAccountGetWallPaper{
		Wallpaper: mtproto.MakeTLInputWallPaperNoFile(&mtproto.InputWallPaper{Id: solid.GetId()}).To_InputWallPaper(),
	})
	if err != nil || got.GetId() != solid.GetId() || got.GetPredicateName() != mtproto.Predicate_wallPaperNoFile {
		t.Fatalf("get solid: %v %#v", err, got)
	}
	if err := got.Encode(mtproto.NewEncodeBuf(512), 232); err != nil {
		t.Fatal(err)
	}
	if _, err := messages.NewWallpapers().AccountGetWallPaper(ctx, &mtproto.TLAccountGetWallPaper{
		Wallpaper: mtproto.MakeTLInputWallPaperNoFile(&mtproto.InputWallPaper{Id: 1}).To_InputWallPaper(),
	}); err == nil {
		t.Fatal("unknown solid id was accepted")
	}
}
