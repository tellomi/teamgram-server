package accountassets

import "github.com/teamgram/proto/mtproto"

// App themes for account.getThemes. One day and one night preset per client
// format. There is no theme file; clients use base_theme and accent_color.
// https://core.telegram.org/method/account.getThemes
var appThemeFormats = []string{"android", "ios", "macos", "tdesktop"}

const appThemeIDBase int64 = 9_200_000_001

type appThemeDef struct {
	name   string
	title  string
	day    bool
	deflt  bool
	accent int32
}

var appThemePresets = []appThemeDef{
	{"day", "Day", true, true, 0x2481cc},
	{"night", "Night", false, false, 0x6ab2f2},
}

func appThemes(format string) []*mtproto.Theme {
	idx := formatIndex(format)
	if idx < 0 {
		return nil
	}
	out := make([]*mtproto.Theme, 0, len(appThemePresets))
	for i, def := range appThemePresets {
		id := appThemeIDBase + int64(idx*len(appThemePresets)+i)
		out = append(out, appTheme(id, format, def))
	}
	return out
}

func appThemeInput(format string, in *mtproto.InputTheme) (*mtproto.Theme, bool, error) {
	if in == nil {
		return nil, false, nil
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputTheme:
		theme, owner := findAppTheme(in.GetId())
		if theme == nil {
			return nil, false, nil
		}
		if theme.GetAccessHash() != in.GetAccessHash() || owner != format {
			return nil, true, mtproto.ErrThemeInvalid
		}
		return theme, true, nil
	case mtproto.Predicate_inputThemeSlug:
		theme, owner := findAppThemeSlug(in.GetSlug())
		if theme == nil {
			return nil, false, nil
		}
		if owner != format {
			return nil, true, mtproto.ErrThemeInvalid
		}
		return theme, true, nil
	default:
		return nil, false, nil
	}
}

func findAppTheme(id int64) (*mtproto.Theme, string) {
	for _, format := range appThemeFormats {
		for _, theme := range appThemes(format) {
			if theme.GetId() == id {
				return theme, format
			}
		}
	}
	return nil, ""
}

func findAppThemeSlug(slug string) (*mtproto.Theme, string) {
	for _, format := range appThemeFormats {
		for _, theme := range appThemes(format) {
			if theme.GetSlug() == slug {
				return theme, format
			}
		}
	}
	return nil, ""
}

func formatIndex(format string) int {
	for i, name := range appThemeFormats {
		if name == format {
			return i
		}
	}
	return -1
}

func appTheme(id int64, format string, def appThemeDef) *mtproto.Theme {
	base := mtproto.MakeTLBaseThemeNight(&mtproto.BaseTheme{}).To_BaseTheme()
	if def.day {
		base = mtproto.MakeTLBaseThemeDay(&mtproto.BaseTheme{}).To_BaseTheme()
	}
	return mtproto.MakeTLTheme(&mtproto.Theme{
		Default:    def.deflt,
		Id:         id,
		AccessHash: accessOf(id),
		Slug:       format + "-" + def.name,
		Title:      def.title,
		Settings: []*mtproto.ThemeSettings{
			mtproto.MakeTLThemeSettings(&mtproto.ThemeSettings{
				BaseTheme:     base,
				AccentColor:   def.accent,
				MessageColors: []int32{def.accent},
			}).To_ThemeSettings(),
		},
	}).To_Theme()
}
