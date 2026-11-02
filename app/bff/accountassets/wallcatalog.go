package accountassets

import (
	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Built-in wallpapers for account.getWallPapers. They use wallPaperNoFile,
// so the client can paint them without a document.
// https://core.telegram.org/method/account.getWallPapers
// https://core.telegram.org/constructor/wallPaperNoFile
//
// Index is the stable id offset from solidWallIDBase. Clients that saved
// 9100000001–9100000006 still resolve those ids; only the fill changed.
// 9100000007 is appended.
//
//	9100000001  light default  #D4E4F7→#8EC3EB @45°  → solid #F2F2F2
//	9100000002  dark default   #0E1621→#1B2836 @45°  → solid #000000
//	9100000003  pink/red       #F8D0D0→#E05A5A       → solid #FFFFFF
//	9100000004  teal           #1ABC9C→#16A085 @90°  → solid #E5E5E5
//	9100000005  purple (dark)  #2C3E50→#8E44AD @135° → #121212→#1E1E1E @180°
//	9100000006  green          #27AE60→#145A32 @180° → solid #CFCFCF
//	9100000007  (new)                                 #F5F5F5→#E6E6E6 @180°
type solidWall struct {
	dark     bool
	def      bool
	color    int32
	second   int32
	gradient bool
	rotation int32
}

var solidWalls = []solidWall{
	{def: true, color: 0xf2f2f2},
	{dark: true, def: true, color: 0x000000},
	{color: 0xffffff},
	{color: 0xe5e5e5},
	{dark: true, color: 0x121212, second: 0x1e1e1e, gradient: true, rotation: 180},
	{color: 0xcfcfcf},
	{color: 0xf5f5f5, second: 0xe6e6e6, gradient: true, rotation: 180},
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
		BackgroundColor: wrapperspb.Int32(def.color),
	}
	// second_background_color and rotation share TL flags.4. A gradient
	// writes both, including a zero rotation. A solid writes neither, or
	// the decoder consumes an extra int and the color shifts.
	if def.gradient {
		settings.SecondBackgroundColor = wrapperspb.Int32(def.second)
		settings.Rotation = wrapperspb.Int32(def.rotation)
	}
	return mtproto.MakeTLWallPaperNoFile(&mtproto.WallPaper{
		Id:       id,
		Default:  def.def,
		Dark:     def.dark,
		Settings: mtproto.MakeTLWallPaperSettings(settings).To_WallPaperSettings(),
	}).To_WallPaper()
}
