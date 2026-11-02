package accountassets

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMonochromeWallpapers(t *testing.T) {
	Reset()
	type want struct {
		id       int64
		dark     bool
		def      bool
		color    int32
		second   int32
		gradient bool
		rotation int32
	}
	wants := []want{
		{solidWallIDBase + 0, false, true, 0xf2f2f2, 0, false, 0},
		{solidWallIDBase + 1, true, true, 0x000000, 0, false, 0},
		{solidWallIDBase + 2, false, false, 0xffffff, 0, false, 0},
		{solidWallIDBase + 3, false, false, 0xe5e5e5, 0, false, 0},
		{solidWallIDBase + 4, true, false, 0x121212, 0x1e1e1e, true, 180},
		{solidWallIDBase + 5, false, false, 0xcfcfcf, 0, false, 0},
		{solidWallIDBase + 6, false, false, 0xf5f5f5, 0xe6e6e6, true, 180},
	}
	papers := builtinWallpapers()
	if len(papers) != len(wants) {
		t.Fatalf("catalog len %d", len(papers))
	}
	for i, row := range wants {
		paper := papers[i]
		if paper.GetId() != row.id || paper.GetDark() != row.dark || paper.GetDefault() != row.def {
			t.Fatalf("meta id %d dark %v default %v", paper.GetId(), paper.GetDark(), paper.GetDefault())
		}
		assertWallFill(t, paper, row.color, row.second, row.gradient, row.rotation)
		assertWallFill(t, roundTripWall(t, paper), row.color, row.second, row.gradient, row.rotation)
		got, err := GetWallPaper(1, mtproto.MakeTLInputWallPaperNoFile(&mtproto.InputWallPaper{Id: row.id}).To_InputWallPaper())
		if err != nil || got.GetId() != row.id {
			t.Fatalf("get %d: %v", row.id, err)
		}
		assertWallFill(t, got, row.color, row.second, row.gradient, row.rotation)
	}
	// Generation 1 was the six tinted walls at version 1. A client that
	// cached that hash must not be told the catalog is unchanged.
	old := make([]idVer, 6)
	for i := range old {
		old[i] = idVer{id: solidWallIDBase + int64(i), ver: 1}
	}
	listed := GetWallPapers(1, 0)
	if listed.GetHash() == 0 || listed.GetHash() == hashPairs(old) {
		t.Fatal("wallpaper hash still matches the colored catalog")
	}
	if GetWallPapers(1, listed.GetHash()).GetPredicateName() != mtproto.Predicate_account_wallPapersNotModified {
		t.Fatal("new wallpaper hash did not cache")
	}
}

func TestMonochromeThemes(t *testing.T) {
	Reset()
	if builtinCatalogVer != 2 {
		t.Fatalf("catalog generation %d, want 2", builtinCatalogVer)
	}
	for _, format := range appThemeFormats {
		themes := appThemes(format)
		if len(themes) != 2 {
			t.Fatalf("%s len %d", format, len(themes))
		}
		day, night := themes[0], themes[1]
		if day.GetSlug() != format+"-day" || !day.GetDefault() || accentOf(t, day, 0) != 0x000000 {
			t.Fatalf("day slug %s accent #%06x", day.GetSlug(), uint32(accentOf(t, day, 0)))
		}
		if night.GetSlug() != format+"-night" || night.GetDefault() || accentOf(t, night, 0) != 0xffffff {
			t.Fatalf("night slug %s accent #%06x", night.GetSlug(), uint32(accentOf(t, night, 0)))
		}
		if day.GetId() != appThemeIDBase+int64(formatIndex(format)*len(appThemePresets)) {
			t.Fatalf("day id moved: %d", day.GetId())
		}
		if night.GetId() != day.GetId()+1 {
			t.Fatalf("night id moved: %d", night.GetId())
		}
		for _, th := range themes {
			assertGreyTheme(t, th)
		}
	}
	oldApp := []idVer{{id: appThemeIDBase, ver: 1}, {id: appThemeIDBase + 1, ver: 1}}
	listed, err := GetThemes(4, "android", 0)
	if err != nil {
		t.Fatal(err)
	}
	if listed.GetHash() == hashPairs(oldApp) {
		t.Fatal("app theme hash still matches generation 1")
	}

	want := []struct {
		slug  string
		light int32
		dark  int32
	}{
		{"day", 0x000000, 0xffffff},
		{"heart", 0x333333, 0xcccccc},
		{"fire", 0x1a1a1a, 0xe6e6e6},
		{"party", 0x4d4d4d, 0xb3b3b3},
		{"star", 0x666666, 0x999999},
		{"like", 0x262626, 0xd9d9d9},
	}
	chats := GetChatThemes(0)
	if len(chats.GetThemes()) != len(want) {
		t.Fatalf("chat themes %d", len(chats.GetThemes()))
	}
	for i, row := range want {
		th := chats.GetThemes()[i]
		if th.GetSlug() != row.slug || th.GetId() != chatThemeIDBase+int64(i) || !th.GetForChat() {
			t.Fatalf("moved slug %s id %d", th.GetSlug(), th.GetId())
		}
		if i == 0 && !th.GetDefault() {
			t.Fatal("day chat theme is not default")
		}
		if accentOf(t, th, 0) != row.light || accentOf(t, th, 1) != row.dark {
			t.Fatalf("%s accents #%06x #%06x", row.slug, uint32(accentOf(t, th, 0)), uint32(accentOf(t, th, 1)))
		}
		assertGreyTheme(t, th)
		round := roundTripTheme(t, th)
		if round.GetSlug() != row.slug || accentOf(t, round, 0) != row.light || accentOf(t, round, 1) != row.dark {
			t.Fatalf("round trip %s", row.slug)
		}
	}
	oldChat := make([]idVer, len(want))
	for i := range oldChat {
		oldChat[i] = idVer{id: chatThemeIDBase + int64(i), ver: 1}
	}
	if chats.GetHash() == 0 || chats.GetHash() == hashPairs(oldChat) {
		t.Fatal("chat theme hash still matches generation 1")
	}
}

func accentOf(t *testing.T, th *mtproto.Theme, i int) int32 {
	t.Helper()
	settings := th.GetSettings()
	if i >= len(settings) || settings[i] == nil {
		t.Fatalf("%s missing settings[%d]", th.GetSlug(), i)
	}
	return settings[i].GetAccentColor()
}

func assertGreyTheme(t *testing.T, th *mtproto.Theme) {
	t.Helper()
	for _, s := range th.GetSettings() {
		if !grey24(s.GetAccentColor()) {
			t.Fatalf("%s accent #%06x", th.GetSlug(), uint32(s.GetAccentColor()))
		}
		for _, c := range s.GetMessageColors() {
			if !grey24(c) {
				t.Fatalf("%s message color #%06x", th.GetSlug(), uint32(c))
			}
		}
	}
}

func assertWallFill(t *testing.T, paper *mtproto.WallPaper, color, second int32, gradient bool, rotation int32) {
	t.Helper()
	st := paper.GetSettings()
	if st == nil || st.GetBackgroundColor() == nil || st.GetBackgroundColor().GetValue() != color || !grey24(color) {
		t.Fatalf("id %d color %#v", paper.GetId(), st)
	}
	if !gradient {
		if st.GetSecondBackgroundColor() != nil || st.GetRotation() != nil {
			t.Fatalf("id %d solid has a gradient", paper.GetId())
		}
		return
	}
	if st.GetSecondBackgroundColor() == nil || st.GetSecondBackgroundColor().GetValue() != second || !grey24(second) {
		t.Fatalf("id %d second %#v", paper.GetId(), st.GetSecondBackgroundColor())
	}
	if st.GetRotation() == nil || st.GetRotation().GetValue() != rotation {
		t.Fatalf("id %d rotation %#v", paper.GetId(), st.GetRotation())
	}
}

func roundTripWall(t *testing.T, paper *mtproto.WallPaper) *mtproto.WallPaper {
	t.Helper()
	buf := mtproto.NewEncodeBuf(512)
	if err := paper.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	decoded := &mtproto.WallPaper{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func roundTripTheme(t *testing.T, th *mtproto.Theme) *mtproto.Theme {
	t.Helper()
	buf := mtproto.NewEncodeBuf(1024)
	if err := th.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	decoded := &mtproto.Theme{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func grey24(c int32) bool {
	u := uint32(c)
	if u&^0xffffff != 0 {
		return false
	}
	r, g, b := (u>>16)&0xff, (u>>8)&0xff, u&0xff
	return r == g && g == b
}
