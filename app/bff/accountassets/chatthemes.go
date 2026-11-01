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
	{"day", "Day", "😀", 0x2481cc, 0x6ab2f2},
	{"heart", "Heart", "❤️", 0xe05a5a, 0xc44d4d},
	{"fire", "Fire", "🔥", 0xe67e22, 0xd35400},
	{"party", "Party", "🎉", 0x8e44ad, 0x6c3483},
	{"star", "Star", "⭐️", 0xf1c40f, 0xb7950b},
	{"like", "Like", "👍", 0x2ecc71, 0x1e8449},
}

const chatThemeIDBase int64 = 9_000_000_001

func chatThemes(hash int64) *mtproto.Account_Themes {
	rows := make([]*mtproto.Theme, 0, len(chatThemeCatalog))
	pairs := make([]idVer, 0, len(chatThemeCatalog))
	for i, def := range chatThemeCatalog {
		id := chatThemeIDBase + int64(i)
		rows = append(rows, chatTheme(id, i == 0, def))
		pairs = append(pairs, idVer{id: id, ver: 1})
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
