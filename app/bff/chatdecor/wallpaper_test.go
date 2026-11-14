package chatdecor

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestRememberAndGet(t *testing.T) {
	paper := mtproto.MakeTLWallPaperNoFile(&mtproto.WallPaper{
		Id: 42,
		Settings: mtproto.MakeTLWallPaperSettings(&mtproto.WallPaperSettings{
			BackgroundColor: mtproto.MakeFlagsInt32(0x336699),
		}).To_WallPaperSettings(),
	}).To_WallPaper()
	Remember(paper)
	got := Get(42)
	if got == nil || got.GetId() != 42 || got.GetSettings().GetBackgroundColor().GetValue() != 0x336699 {
		t.Fatalf("wallpaper %#v", got)
	}
	if Get(0) != nil {
		t.Fatal("zero id should be empty")
	}
}
