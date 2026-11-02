package accountassets

import (
	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Built-in chat themes. A theme has no document: clients apply the emoticon
// and the day/night accent colors.
// https://core.telegram.org/api/themes
// https://core.telegram.org/constructor/theme
type chatThemeDef struct {
	slug       string
	title      string
	emoticon   string
	accent     int32
	darkAccent int32
}

var chatThemeCatalog = []chatThemeDef{
	// Slugs and emoticons are the stable keys (messages.setChatTheme).
	// Accents are greyscale. The day pair replaces Telegram blue
	// 0x2481cc / 0x6ab2f2. Light accent is for baseThemeDay, dark accent
	// for baseThemeNight.
	//
	//	day    0x2481cc / 0x6ab2f2 → 0x000000 / 0xFFFFFF
	//	heart  0xE05A5A / 0xC44D4D → 0x333333 / 0xCCCCCC
	//	fire   0xE67E22 / 0xD35400 → 0x1A1A1A / 0xE6E6E6
	//	party  0x8E44AD / 0x6C3483 → 0x4D4D4D / 0xB3B3B3
	//	star   0xF1C40F / 0xB7950B → 0x666666 / 0x999999
	//	like   0x2ECC71 / 0x1E8449 → 0x262626 / 0xD9D9D9
	{"day", "Day", "😀", 0x000000, 0xffffff},
	{"heart", "Heart", "❤️", 0x333333, 0xcccccc},
	{"fire", "Fire", "🔥", 0x1a1a1a, 0xe6e6e6},
	{"party", "Party", "🎉", 0x4d4d4d, 0xb3b3b3},
	{"star", "Star", "⭐️", 0x666666, 0x999999},
	{"like", "Like", "👍", 0x262626, 0xd9d9d9},
}

const chatThemeIDBase int64 = 9_000_000_001

func chatThemes(hash int64) *mtproto.Account_Themes {
	rows := make([]*mtproto.Theme, 0, len(chatThemeCatalog))
	pairs := make([]idVer, 0, len(chatThemeCatalog))
	for i, def := range chatThemeCatalog {
		id := chatThemeIDBase + int64(i)
		rows = append(rows, chatTheme(id, i == 0, def))
		pairs = append(pairs, idVer{id: id, ver: builtinCatalogVer})
	}
	sum := hashPairs(pairs)
	if notModified(hash, sum) {
		return themesNotModified()
	}
	return themesReply(sum, rows)
}

func chatTheme(id int64, deflt bool, def chatThemeDef) *mtproto.Theme {
	return mtproto.MakeTLTheme(&mtproto.Theme{
		Default:    deflt,
		ForChat:    true,
		Id:         id,
		AccessHash: accessOf(id),
		Slug:       def.slug,
		Title:      def.title,
		Emoticon:   wrapperspb.String(def.emoticon),
		Settings: []*mtproto.ThemeSettings{
			chatAccent(true, def.accent),
			chatAccent(false, def.darkAccent),
		},
	}).To_Theme()
}

func chatAccent(day bool, accent int32) *mtproto.ThemeSettings {
	base := mtproto.MakeTLBaseThemeNight(&mtproto.BaseTheme{}).To_BaseTheme()
	if day {
		base = mtproto.MakeTLBaseThemeDay(&mtproto.BaseTheme{}).To_BaseTheme()
	}
	return mtproto.MakeTLThemeSettings(&mtproto.ThemeSettings{
		BaseTheme:     base,
		AccentColor:   accent,
		MessageColors: []int32{accent},
	}).To_ThemeSettings()
}
