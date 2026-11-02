package inmem

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
	dayApp := appThemes.GetThemes()[0]
	nightApp := appThemes.GetThemes()[1]
	if dayApp.GetSlug() != "android-day" || dayApp.GetSettings()[0].GetAccentColor() != 0x000000 {
		t.Fatalf("day app theme slug %s accent #%06x", dayApp.GetSlug(), uint32(dayApp.GetSettings()[0].GetAccentColor()))
	}
	if nightApp.GetSlug() != "android-night" || nightApp.GetSettings()[0].GetAccentColor() != 0xffffff {
		t.Fatalf("night app theme slug %s accent #%06x", nightApp.GetSlug(), uint32(nightApp.GetSettings()[0].GetAccentColor()))
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
	if picked.GetSlug() != "day" || picked.GetEmoticon().GetValue() == "" || len(picked.GetSettings()) != 2 ||
		picked.GetSettings()[0].GetAccentColor() != 0x000000 || picked.GetSettings()[1].GetAccentColor() != 0xffffff {
		t.Fatalf("picked theme slug %s accents #%06x #%06x", picked.GetSlug(),
			uint32(picked.GetSettings()[0].GetAccentColor()), uint32(picked.GetSettings()[1].GetAccentColor()))
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
	var lightDef, darkDef bool
	for _, paper := range walls.GetWallpapers() {
		if paper.GetPredicateName() != mtproto.Predicate_wallPaperNoFile || paper.GetSettings() == nil || paper.GetSettings().GetBackgroundColor() == nil {
			continue
		}
		st := paper.GetSettings()
		if !greyChannel(st.GetBackgroundColor().GetValue()) {
			t.Fatalf("wallpaper %d color #%06x", paper.GetId(), uint32(st.GetBackgroundColor().GetValue()))
		}
		if sec := st.GetSecondBackgroundColor(); sec != nil && !greyChannel(sec.GetValue()) {
			t.Fatalf("wallpaper %d second #%06x", paper.GetId(), uint32(sec.GetValue()))
		}
		if paper.GetDefault() && !paper.GetDark() {
			lightDef = true
			if st.GetBackgroundColor().GetValue() != 0xf2f2f2 || st.GetSecondBackgroundColor() != nil {
				t.Fatalf("light default #%06x", uint32(st.GetBackgroundColor().GetValue()))
			}
		}
		if paper.GetDefault() && paper.GetDark() {
			darkDef = true
			if st.GetBackgroundColor().GetValue() != 0x000000 || st.GetSecondBackgroundColor() != nil {
				t.Fatalf("dark default #%06x", uint32(st.GetBackgroundColor().GetValue()))
			}
		}
		if solid == nil {
			solid = paper
		}
	}
	if solid == nil || !lightDef || !darkDef {
		t.Fatal("missing default solid wallpapers")
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
	// Ids issued before the monochrome catalog still resolve.
	legacy, err := messages.NewWallpapers().AccountGetWallPaper(ctx, &mtproto.TLAccountGetWallPaper{
		Wallpaper: mtproto.MakeTLInputWallPaperNoFile(&mtproto.InputWallPaper{Id: 9_100_000_001}).To_InputWallPaper(),
	})
	if err != nil || legacy.GetId() != 9_100_000_001 || legacy.GetSettings().GetBackgroundColor().GetValue() != 0xf2f2f2 {
		t.Fatalf("legacy light default: %v %#v", err, legacy)
	}
}

func greyChannel(c int32) bool {
	u := uint32(c)
	if u&^0xffffff != 0 {
		return false
	}
	r, g, b := (u>>16)&0xff, (u>>8)&0xff, u&0xff
	return r == g && g == b
}
