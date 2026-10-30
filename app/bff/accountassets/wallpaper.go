package accountassets

import (
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

type wallRec struct {
	id       int64
	access   int64
	owner    int64
	slug     string
	pattern  bool
	doc      *mtproto.Document
	settings *mtproto.WallPaperSettings
	version  int64
}

// UploadWallPaper implements account.uploadWallPaper.
// https://core.telegram.org/method/account.uploadWallPaper
func UploadWallPaper(self int64, in *mtproto.TLAccountUploadWallPaper) (*mtproto.WallPaper, error) {
	return shared().uploadWall(self, in)
}

func (s *store) uploadWall(self int64, in *mtproto.TLAccountUploadWallPaper) (*mtproto.WallPaper, error) {
	if in == nil {
		return nil, mtproto.ErrWallpaperFileInvalid
	}
	if err := fileError(in.GetFile()); err != nil {
		return nil, mtproto.ErrWallpaperFileInvalid
	}
	mime := strings.ToLower(in.GetMimeType())
	if !wallMIME[mime] {
		return nil, mtproto.ErrWallpaperMimeInvalid
	}
	if !settingsOK(in.GetSettings()) {
		return nil, mtproto.ErrWallpaperInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	name := in.GetFile().GetName()
	if name == "" {
		name = "wallpaper"
	}
	doc := s.putDoc(self, "wall", mime, name, in.GetFile().GetParts(), false)
	id := s.nextID()
	pattern := mime == "application/x-tgwallpattern"
	if mime == "image/png" && in.GetSettings() != nil {
		st := in.GetSettings()
		if st.GetBackgroundColor() != nil || st.GetIntensity() != nil {
			pattern = true
		}
	}
	w := &wallRec{
		id: id, access: accessOf(id), owner: self,
		slug:     freeSlug(s.wallSlug, "w"+strconv.FormatInt(id, 36)),
		pattern:  pattern,
		doc:      doc.doc,
		settings: copyWallSettings(in.GetSettings()),
		version:  1,
	}
	s.walls[id] = w
	s.wallSlug[w.slug] = id
	_ = in.GetForChat()
	return w.view(self), nil
}

// GetWallPapers implements account.getWallPapers.
// https://core.telegram.org/method/account.getWallPapers
func GetWallPapers(self int64, hash int64) *mtproto.Account_WallPapers {
	return shared().wallList(self, hash)
}

func (s *store) wallList(self int64, hash int64) *mtproto.Account_WallPapers {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[self]
	var pairs []idVer
	var rows []*wallRec
	if u != nil {
		for id := range u.savedWalls {
			w := s.walls[id]
			if w == nil {
				continue
			}
			rows = append(rows, w)
			pairs = append(pairs, idVer{id: w.id, ver: w.version})
		}
	}
	sum := hashPairs(pairs)
	if notModified(hash, sum) {
		return mtproto.MakeTLAccountWallPapersNotModified(&mtproto.Account_WallPapers{}).To_Account_WallPapers()
	}
	sortWalls(rows)
	out := make([]*mtproto.WallPaper, 0, len(rows))
	for _, w := range rows {
		out = append(out, w.view(self))
	}
	return mtproto.MakeTLAccountWallPapers(&mtproto.Account_WallPapers{
		Hash:       sum,
		Wallpapers: out,
	}).To_Account_WallPapers()
}

func sortWalls(rows []*wallRec) {
	for i := 1; i < len(rows); i++ {
		j := i
		for j > 0 && rows[j].id < rows[j-1].id {
			rows[j], rows[j-1] = rows[j-1], rows[j]
			j--
		}
	}
}

// GetWallPaper implements account.getWallPaper.
// https://core.telegram.org/method/account.getWallPaper
func GetWallPaper(self int64, in *mtproto.InputWallPaper) (*mtproto.WallPaper, error) {
	return shared().getWall(self, in)
}

func (s *store) getWall(self int64, in *mtproto.InputWallPaper) (*mtproto.WallPaper, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.wallByInput(in)
	if err != nil {
		return nil, err
	}
	return w.view(self), nil
}

// GetMultiWallPapers implements account.getMultiWallPapers.
// https://core.telegram.org/method/account.getMultiWallPapers
func GetMultiWallPapers(self int64, in []*mtproto.InputWallPaper) (*mtproto.Vector_WallPaper, error) {
	return shared().multiWalls(self, in)
}

func (s *store) multiWalls(self int64, in []*mtproto.InputWallPaper) (*mtproto.Vector_WallPaper, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*mtproto.WallPaper, 0, len(in))
	for _, item := range in {
		w, err := s.wallByInput(item)
		if err != nil {
			return nil, err
		}
		out = append(out, w.view(self))
	}
	return &mtproto.Vector_WallPaper{Datas: out}, nil
}

// SaveWallPaper implements account.saveWallPaper.
// https://core.telegram.org/method/account.saveWallPaper
func SaveWallPaper(self int64, in *mtproto.TLAccountSaveWallPaper) (*mtproto.Bool, error) {
	return shared().saveWall(self, in)
}

func (s *store) saveWall(self int64, in *mtproto.TLAccountSaveWallPaper) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrWallpaperInvalid
	}
	if !settingsOK(in.GetSettings()) {
		return nil, mtproto.ErrWallpaperInvalid
	}
	return s.rememberWall(self, in.GetWallpaper(), in.GetSettings(), mtproto.FromBool(in.GetUnsave()), false)
}

// InstallWallPaper implements account.installWallPaper.
// Installing also saves the wallpaper.
// https://core.telegram.org/method/account.installWallPaper
func InstallWallPaper(self int64, in *mtproto.TLAccountInstallWallPaper) (*mtproto.Bool, error) {
	return shared().installWall(self, in)
}

func (s *store) installWall(self int64, in *mtproto.TLAccountInstallWallPaper) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrWallpaperInvalid
	}
	if !settingsOK(in.GetSettings()) {
		return nil, mtproto.ErrWallpaperInvalid
	}
	return s.rememberWall(self, in.GetWallpaper(), in.GetSettings(), false, true)
}

func (s *store) rememberWall(self int64, in *mtproto.InputWallPaper, settings *mtproto.WallPaperSettings, unsave, install bool) (*mtproto.Bool, error) {
	if in != nil && in.GetPredicateName() == mtproto.Predicate_inputWallPaperNoFile {
		return nil, mtproto.ErrWallpaperInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.wallByInput(in)
	if err != nil {
		return nil, err
	}
	if settings != nil {
		w.settings = copyWallSettings(settings)
		w.version++
	}
	u := s.user(self)
	if unsave {
		delete(u.savedWalls, w.id)
		if u.installedWall == w.id {
			u.installedWall = 0
		}
		return mtproto.BoolTrue, nil
	}
	u.savedWalls[w.id] = struct{}{}
	if install {
		u.installedWall = w.id
	}
	return mtproto.BoolTrue, nil
}

// ResetWallPapers implements account.resetWallPapers.
// https://core.telegram.org/method/account.resetWallPapers
func ResetWallPapers(self int64) *mtproto.Bool {
	s := shared()
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.users[self]; u != nil {
		u.savedWalls = map[int64]struct{}{}
		u.installedWall = 0
	}
	return mtproto.BoolTrue
}

func (s *store) wallByInput(in *mtproto.InputWallPaper) (*wallRec, error) {
	if in == nil {
		return nil, mtproto.ErrWallpaperInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputWallPaper:
		w := s.walls[in.GetId()]
		if w == nil || w.access != in.GetAccessHash() {
			return nil, mtproto.ErrWallpaperInvalid
		}
		return w, nil
	case mtproto.Predicate_inputWallPaperSlug:
		if in.GetSlug() == "" {
			return nil, mtproto.ErrWallpaperInvalid
		}
		id, ok := s.wallSlug[in.GetSlug()]
		if !ok {
			return nil, mtproto.ErrWallpaperInvalid
		}
		return s.walls[id], nil
	default:
		return nil, mtproto.ErrWallpaperInvalid
	}
}

func (s *store) wallpaperObject(self int64, in *mtproto.InputWallPaper, settings *mtproto.WallPaperSettings) (*mtproto.WallPaper, error) {
	if !settingsOK(settings) {
		return nil, mtproto.ErrWallpaperInvalid
	}
	if in.GetPredicateName() == mtproto.Predicate_inputWallPaperNoFile {
		paper := mtproto.MakeTLWallPaperNoFile(&mtproto.WallPaper{
			Id:       in.GetId(),
			Settings: copyWallSettings(settings),
		}).To_WallPaper()
		return paper, nil
	}
	w, err := s.wallByInput(in)
	if err != nil {
		return nil, err
	}
	paper := w.view(self)
	if settings != nil {
		paper.Settings = copyWallSettings(settings)
	}
	return paper, nil
}

func (w *wallRec) view(self int64) *mtproto.WallPaper {
	out := &mtproto.WallPaper{
		Id:         w.id,
		Creator:    self == w.owner,
		Pattern:    w.pattern,
		AccessHash: w.access,
		Slug:       w.slug,
		Document:   cloneDoc(w.doc),
	}
	if w.settings != nil {
		out.Settings = proto.Clone(w.settings).(*mtproto.WallPaperSettings)
	}
	return mtproto.MakeTLWallPaper(out).To_WallPaper()
}
