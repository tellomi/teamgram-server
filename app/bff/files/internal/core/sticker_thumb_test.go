package core

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
)

func TestStickerSetThumbWithoutPlugin(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "")
	downloadFile = func(context.Context, *mtproto.InputFileLocation, int64, int32) (*mtproto.Upload_File, error) {
		t.Fatal("sticker set thumb asked dfs")
		return nil, nil
	}
	t.Cleanup(func() { downloadFile = nil })

	c := fileCore()
	set := mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{
		ShortName: catalog.ShortTwemoji,
	}).To_InputStickerSet()
	got, err := c.UploadGetFile(&mtproto.TLUploadGetFile{
		Location: mtproto.MakeTLInputStickerSetThumb(&mtproto.InputFileLocation{
			Stickerset:   set,
			ThumbVersion: 1,
		}).To_InputFileLocation(),
		Limit: 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetBytes()) == 0 {
		t.Fatal("empty sticker set thumb")
	}
	if got.GetType().GetPredicateName() == "" {
		t.Fatal("missing file type")
	}

	_, err = c.UploadGetFile(&mtproto.TLUploadGetFile{
		Location: mtproto.MakeTLInputStickerSetThumb(&mtproto.InputFileLocation{
			Stickerset: mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{
				ShortName: "not-a-real-set",
			}).To_InputStickerSet(),
		}).To_InputFileLocation(),
		Limit: 32,
	})
	if err == nil || strings.Contains(err.Error(), "ENTERPRISE_IS_BLOCKED") {
		t.Fatalf("unknown set: %v", err)
	}
}
