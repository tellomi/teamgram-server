package accountassets

import (
	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Solid wallpapers shipped with account.getWallPapers. They use
// wallPaperNoFile, so the client can paint them without a document.
// https://core.telegram.org/method/account.getWallPapers
// https://core.telegram.org/constructor/wallPaperNoFile
type solidWall struct {
	dark     bool
	def      bool
	color    int32
	second   int32
	rotation int32
}

var solidWalls = []solidWall{
	{false, true, 0xd4e4f7, 0x8ec3eb, 45},
	{true, true, 0x0e1621, 0x1b2836, 45},
	{false, false, 0xf8d0d0, 0xe05a5a, 0},
	{false, false, 0x1abc9c, 0x16a085, 90},
	{true, false, 0x2c3e50, 0x8e44ad, 135},
	{false, false, 0x27ae60, 0x145a32, 180},
}

const solidWallIDBase int64 = 9_100_000_001

func builtinWallpapers() []*mtproto.WallPaper {
	out := make([]*mtproto.WallPaper, 0, len(solidWalls))
	for i, def := range solidWalls {
		out = append(out, solidWallpaper(solidWallIDBase+int64(i), def))
	}
	return out
}

func builtinWall(id int64) *mtproto.WallPaper {
	i := id - solidWallIDBase
	if i < 0 || int(i) >= len(solidWalls) {
		return nil
	}
	return solidWallpaper(id, solidWalls[i])
}

func builtinInput(in *mtproto.InputWallPaper) (*mtproto.WallPaper, bool, error) {
	if in == nil || in.GetPredicateName() != mtproto.Predicate_inputWallPaperNoFile {
		return nil, false, nil
	}
	paper := builtinWall(in.GetId())
	if paper == nil {
		return nil, true, mtproto.ErrWallpaperInvalid
	}
	return paper, true, nil
}

func solidWallpaper(id int64, def solidWall) *mtproto.WallPaper {
	settings := &mtproto.WallPaperSettings{
		BackgroundColor:       wrapperspb.Int32(def.color),
		SecondBackgroundColor: wrapperspb.Int32(def.second),
	}
	if def.rotation != 0 {
		settings.Rotation = wrapperspb.Int32(def.rotation)
	}
	return mtproto.MakeTLWallPaperNoFile(&mtproto.WallPaper{
		Id:       id,
		Default:  def.def,
		Dark:     def.dark,
		Settings: mtproto.MakeTLWallPaperSettings(settings).To_WallPaperSettings(),
	}).To_WallPaper()
}
