package accountassets

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// https://core.telegram.org/method/account.createTheme
// A slug may be empty (the server mints one). A provided slug is [A-Za-z0-9_].
var slugRE = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

type themeRec struct {
	id       int64
	access   int64
	owner    int64
	slug     string
	title    string
	format   string
	doc      *mtproto.Document
	settings []*mtproto.ThemeSettings
	version  int64
	saves    int32
}

func slugOK(s string) bool { return slugRE.MatchString(s) }

func titleOK(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && utf8.RuneCountInString(s) <= 128
}

// UploadTheme implements account.uploadTheme.
// https://core.telegram.org/method/account.uploadTheme
func UploadTheme(self int64, in *mtproto.TLAccountUploadTheme) (*mtproto.Document, error) {
	return shared().uploadTheme(self, in)
}

func (s *store) uploadTheme(self int64, in *mtproto.TLAccountUploadTheme) (*mtproto.Document, error) {
	if in == nil {
		return nil, mtproto.ErrThemeFileInvalid
	}
	if err := fileError(in.GetFile()); err != nil {
		return nil, mtproto.ErrThemeFileInvalid
	}
	mime := in.GetMimeType()
	if _, ok := themeMIMEFormat[mime]; !ok {
		return nil, mtproto.ErrThemeMimeInvalid
	}
	name := strings.TrimSpace(in.GetFileName())
	if name == "" {
		return nil, mtproto.ErrThemeFileInvalid
	}
	if thumb := in.GetThumb(); thumb != nil {
		if err := fileError(thumb); err != nil {
			return nil, mtproto.ErrThemeFileInvalid
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.putDoc(self, "theme", mime, name, in.GetFile().GetParts(), false)
	return cloneDoc(rec.doc), nil
}

// CreateTheme implements account.createTheme.
// https://core.telegram.org/method/account.createTheme
func CreateTheme(self int64, in *mtproto.TLAccountCreateTheme) (*mtproto.Theme, error) {
	return shared().createTheme(self, in)
}

func (s *store) createTheme(self int64, in *mtproto.TLAccountCreateTheme) (*mtproto.Theme, error) {
	if in == nil || !titleOK(in.GetTitle()) {
		return nil, mtproto.ErrThemeTitleInvalid
	}
	slug := strings.TrimSpace(in.GetSlug())
	if slug != "" && !slugOK(slug) {
		return nil, mtproto.ErrThemeSlugInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var doc *mtproto.Document
	format := ""
	if in.GetDocument() != nil {
		rec, err := s.docFor(self, in.GetDocument(), "theme")
		if err != nil {
			return nil, mtproto.ErrThemeFileInvalid
		}
		format = themeMIMEFormat[rec.mime]
		if format == "" {
			return nil, mtproto.ErrThemeMimeInvalid
		}
		doc = rec.doc
	}
	settings, err := s.themeSettings(self, in.GetSettings())
	if err != nil {
		return nil, err
	}
	id := s.nextID()
	if slug == "" {
		slug = freeSlug(s.themeSlug, "t"+strconv.FormatInt(id, 36))
	} else if _, ok := s.themeSlug[slug]; ok {
		return nil, mtproto.ErrThemeSlugOccupied
	}
	th := &themeRec{
		id: id, access: accessOf(id), owner: self, slug: slug,
		title: strings.TrimSpace(in.GetTitle()), format: format, doc: doc,
		settings: settings, version: 1,
	}
	s.themes[id] = th
	s.themeSlug[slug] = id
	return th.view(self), nil
}

func freeSlug(used map[string]int64, base string) string {
	slug := base
	for i := 0; ; i++ {
		if _, ok := used[slug]; !ok && slugOK(slug) {
			return slug
		}
		slug = base + "x" + strconv.Itoa(i)
	}
}

// UpdateTheme implements account.updateTheme.
// https://core.telegram.org/method/account.updateTheme
func UpdateTheme(self int64, in *mtproto.TLAccountUpdateTheme) (*mtproto.Theme, error) {
	return shared().updateTheme(self, in)
}

func (s *store) updateTheme(self int64, in *mtproto.TLAccountUpdateTheme) (*mtproto.Theme, error) {
	if in == nil || !formatOK(in.GetFormat()) {
		return nil, mtproto.ErrThemeFormatInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	th, err := s.themeByInput(in.GetTheme())
	if err != nil {
		return nil, err
	}
	if th.owner != self {
		return nil, mtproto.ErrThemeInvalid
	}
	if th.format != "" && th.format != in.GetFormat() && in.GetDocument() == nil {
		return nil, mtproto.ErrThemeInvalid
	}
	if in.GetSlug() != nil {
		slug := strings.TrimSpace(in.GetSlug().GetValue())
		if !slugOK(slug) {
			return nil, mtproto.ErrThemeSlugInvalid
		}
		if other, ok := s.themeSlug[slug]; ok && other != th.id {
			return nil, mtproto.ErrThemeSlugOccupied
		}
		delete(s.themeSlug, th.slug)
		th.slug = slug
		s.themeSlug[slug] = th.id
	}
	if in.GetTitle() != nil {
		title := strings.TrimSpace(in.GetTitle().GetValue())
		if !titleOK(title) {
			return nil, mtproto.ErrThemeTitleInvalid
		}
		th.title = title
	}
	if in.GetDocument() != nil {
		rec, err := s.docFor(self, in.GetDocument(), "theme")
		if err != nil {
			return nil, mtproto.ErrThemeFileInvalid
		}
		format := themeMIMEFormat[rec.mime]
		if format == "" || format != in.GetFormat() {
			return nil, mtproto.ErrThemeMimeInvalid
		}
		th.doc = rec.doc
		th.format = format
	}
	if in.GetSettings() != nil {
		settings, err := s.themeSettings(self, in.GetSettings())
		if err != nil {
			return nil, err
		}
		th.settings = settings
	}
	th.version++
	return th.view(self), nil
}

// GetTheme implements account.getTheme.
// https://core.telegram.org/method/account.getTheme
func GetTheme(self int64, format string, in *mtproto.InputTheme) (*mtproto.Theme, error) {
	return shared().getTheme(self, format, in)
}

func (s *store) getTheme(self int64, format string, in *mtproto.InputTheme) (*mtproto.Theme, error) {
	if !formatOK(format) {
		return nil, mtproto.ErrThemeFormatInvalid
	}
	if in != nil && in.GetPredicateName() == mtproto.Predicate_inputThemeSlug && !slugOK(in.GetSlug()) {
		return nil, mtproto.ErrThemeSlugInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	th, err := s.themeByInput(in)
	if err != nil {
		return nil, err
	}
	if th.format != "" && th.format != format {
		return nil, mtproto.ErrThemeInvalid
	}
	return th.view(self), nil
}

// GetThemes implements account.getThemes.
// https://core.telegram.org/method/account.getThemes
func GetThemes(self int64, format string, hash int64) (*mtproto.Account_Themes, error) {
	return shared().getThemes(self, format, hash)
}

func (s *store) getThemes(self int64, format string, hash int64) (*mtproto.Account_Themes, error) {
	if !formatOK(format) {
		return nil, mtproto.ErrThemeFormatInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.themeList(self, format, hash), nil
}

// GetChatThemes implements account.getChatThemes.
// https://core.telegram.org/method/account.getChatThemes
// Chat themes are the server emoticon catalog. This process has none.
func GetChatThemes(hash int64) *mtproto.Account_Themes {
	if notModified(hash, EmptyHash) {
		return themesNotModified()
	}
	return themesReply(EmptyHash, nil)
}

// SaveTheme implements account.saveTheme.
// https://core.telegram.org/method/account.saveTheme
func SaveTheme(self int64, in *mtproto.InputTheme, unsave bool) (*mtproto.Bool, error) {
	return shared().saveTheme(self, in, unsave)
}

func (s *store) saveTheme(self int64, in *mtproto.InputTheme, unsave bool) (*mtproto.Bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	th, err := s.themeByInput(in)
	if err != nil {
		return nil, err
	}
	u := s.user(self)
	if unsave {
		if _, ok := u.savedThemes[th.id]; ok {
			delete(u.savedThemes, th.id)
			if th.saves > 0 {
				th.saves--
			}
		}
		return mtproto.BoolTrue, nil
	}
	if _, ok := u.savedThemes[th.id]; !ok {
		u.savedThemes[th.id] = struct{}{}
		th.saves++
	}
	return mtproto.BoolTrue, nil
}

// InstallTheme implements account.installTheme.
// https://core.telegram.org/method/account.installTheme
func InstallTheme(self int64, in *mtproto.TLAccountInstallTheme) (*mtproto.Bool, error) {
	return shared().installTheme(self, in)
}

func (s *store) installTheme(self int64, in *mtproto.TLAccountInstallTheme) (*mtproto.Bool, error) {
	if in == nil {
		return mtproto.BoolTrue, nil
	}
	format := ""
	if in.GetFormat() != nil {
		format = in.GetFormat().GetValue()
		if !formatOK(format) {
			return nil, mtproto.ErrThemeFormatInvalid
		}
	}
	if in.GetBaseTheme() != nil && !baseOK(in.GetBaseTheme()) {
		return nil, mtproto.ErrThemeInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var id int64
	if in.GetTheme() != nil {
		th, err := s.themeByInput(in.GetTheme())
		if err != nil {
			return nil, err
		}
		if format != "" && th.format != "" && th.format != format {
			return nil, mtproto.ErrThemeInvalid
		}
		if format == "" {
			format = th.format
		}
		id = th.id
	}
	if format == "" {
		format = "any"
	}
	key := format
	if in.GetDark() {
		key += "/dark"
	}
	s.user(self).installed[key] = id
	return mtproto.BoolTrue, nil
}

// UniqueGiftChatThemes implements account.getUniqueGiftChatThemes.
// https://core.telegram.org/method/account.getUniqueGiftChatThemes
// No collectible gifts are hosted here, so the page is empty and has no next_offset.
func UniqueGiftChatThemes(offset string, limit int32, hash int64) *mtproto.Account_ChatThemes {
	_ = offset
	_ = limit
	if notModified(hash, EmptyHash) {
		return mtproto.MakeTLAccountChatThemesNotModified(&mtproto.Account_ChatThemes{}).To_Account_ChatThemes()
	}
	return mtproto.MakeTLAccountChatThemes(&mtproto.Account_ChatThemes{
		Hash:   EmptyHash,
		Themes: []*mtproto.ChatTheme{},
		Chats:  []*mtproto.Chat{},
		Users:  []*mtproto.User{},
	}).To_Account_ChatThemes()
}

func (s *store) themeList(self int64, format string, hash int64) *mtproto.Account_Themes {
	u := s.users[self]
	var pairs []idVer
	var rows []*themeRec
	if u != nil {
		for id := range u.savedThemes {
			th := s.themes[id]
			if th == nil || (th.format != "" && th.format != format) {
				continue
			}
			rows = append(rows, th)
			pairs = append(pairs, idVer{id: th.id, ver: th.version})
		}
	}
	sum := hashPairs(pairs)
	if notModified(hash, sum) {
		return themesNotModified()
	}
	sortThemes(rows)
	out := make([]*mtproto.Theme, 0, len(rows))
	for _, th := range rows {
		out = append(out, th.view(self))
	}
	return themesReply(sum, out)
}

func sortThemes(rows []*themeRec) {
	for i := 1; i < len(rows); i++ {
		j := i
		for j > 0 && rows[j].id < rows[j-1].id {
			rows[j], rows[j-1] = rows[j-1], rows[j]
			j--
		}
	}
}

func (s *store) themeByInput(in *mtproto.InputTheme) (*themeRec, error) {
	if in == nil {
		return nil, mtproto.ErrThemeInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputTheme:
		th := s.themes[in.GetId()]
		if th == nil || th.access != in.GetAccessHash() {
			return nil, mtproto.ErrThemeInvalid
		}
		return th, nil
	case mtproto.Predicate_inputThemeSlug:
		id, ok := s.themeSlug[in.GetSlug()]
		if !ok {
			return nil, mtproto.ErrThemeInvalid
		}
		return s.themes[id], nil
	default:
		return nil, mtproto.ErrThemeInvalid
	}
}

func (th *themeRec) view(self int64) *mtproto.Theme {
	out := &mtproto.Theme{
		Creator:    self == th.owner,
		Id:         th.id,
		AccessHash: th.access,
		Slug:       th.slug,
		Title:      th.title,
		Document:   cloneDoc(th.doc),
	}
	if len(th.settings) > 0 {
		out.Settings = make([]*mtproto.ThemeSettings, len(th.settings))
		for i, row := range th.settings {
			out.Settings[i] = proto.Clone(row).(*mtproto.ThemeSettings)
		}
	}
	if th.saves > 0 {
		out.InstallsCount = wrapperspb.Int32(th.saves)
	}
	return mtproto.MakeTLTheme(out).To_Theme()
}

func (s *store) themeSettings(self int64, in []*mtproto.InputThemeSettings) ([]*mtproto.ThemeSettings, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]*mtproto.ThemeSettings, 0, len(in))
	for _, row := range in {
		if row == nil || !baseOK(row.GetBaseTheme()) {
			return nil, mtproto.ErrThemeInvalid
		}
		base := proto.Clone(row.GetBaseTheme()).(*mtproto.BaseTheme)
		item := &mtproto.ThemeSettings{
			MessageColorsAnimated: row.GetMessageColorsAnimated(),
			BaseTheme:             base,
			AccentColor:           row.GetAccentColor(),
		}
		if row.GetOutboxAccentColor() != nil {
			item.OutboxAccentColor = wrapperspb.Int32(row.GetOutboxAccentColor().GetValue())
		}
		if colors := row.GetMessageColors(); len(colors) > 0 {
			item.MessageColors = append([]int32(nil), colors...)
		}
		if wall := row.GetWallpaper(); wall != nil {
			paper, err := s.wallpaperObject(self, wall, row.GetWallpaperSettings())
			if err != nil {
				return nil, err
			}
			item.Wallpaper = paper
		}
		out = append(out, mtproto.MakeTLThemeSettings(item).To_ThemeSettings())
	}
	return out, nil
}
