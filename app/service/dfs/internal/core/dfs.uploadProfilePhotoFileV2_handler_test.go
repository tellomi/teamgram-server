package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

func inputFile(id int64, parts int32, name string) *mtproto.InputFile {
	return mtproto.MakeTLInputFile(&mtproto.InputFile{
		Id_INT64: id,
		Parts:    parts,
		Name:     name,
	}).To_InputFile()
}

func emojiMarkup(id int64, colors ...int32) *mtproto.VideoSize {
	return mtproto.MakeTLVideoSizeEmojiMarkup(&mtproto.VideoSize{
		EmojiId:          id,
		BackgroundColors: colors,
	}).To_VideoSize()
}

func TestProfilePhotoSource(t *testing.T) {
	photo := inputFile(1, 1, "a.jpg")
	video := inputFile(2, 1, "a.mp4")
	markup := emojiMarkup(9, 1, 2)

	kind, src, err := profilePhotoSource(&dfs.TLDfsUploadProfilePhotoFileV2{File: photo})
	if err != nil || kind != profilePhotoFile || src != photo {
		t.Fatalf("photo kind=%d src=%p err=%v", kind, src, err)
	}

	kind, src, err = profilePhotoSource(&dfs.TLDfsUploadProfilePhotoFileV2{Video: video})
	if err != nil || kind != profilePhotoVideo || src != video {
		t.Fatalf("video kind=%d src=%p err=%v", kind, src, err)
	}

	kind, src, err = profilePhotoSource(&dfs.TLDfsUploadProfilePhotoFileV2{File: photo, Video: video})
	if err != nil || kind != profilePhotoVideo || src != video {
		t.Fatalf("video+file kind=%d src=%p err=%v", kind, src, err)
	}

	kind, src, err = profilePhotoSource(&dfs.TLDfsUploadProfilePhotoFileV2{VideoEmojiMarkup: markup})
	if err != nil || kind != profilePhotoEmoji || src != nil {
		t.Fatalf("emoji kind=%d src=%v err=%v", kind, src, err)
	}

	if _, _, err = profilePhotoSource(&dfs.TLDfsUploadProfilePhotoFileV2{}); err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("empty: %v", err)
	}
	if _, _, err = profilePhotoSource(nil); err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("nil: %v", err)
	}
}

func TestUploadProfilePhotoFileV2DoesNotPanic(t *testing.T) {
	c := New(context.Background(), nil)

	_, err := c.DfsUploadProfilePhotoFileV2(&dfs.TLDfsUploadProfilePhotoFileV2{
		Creator: 7,
		File:    inputFile(1, 0, "a.jpg"),
	})
	if err != mtproto.ErrFilePartsInvalid {
		t.Fatalf("photo: %v", err)
	}

	// Video-only used to take the file==nil branch and read file.Parts.
	_, err = c.DfsUploadProfilePhotoFileV2(&dfs.TLDfsUploadProfilePhotoFileV2{
		Creator: 7,
		Video:   inputFile(2, 0, "a.mp4"),
	})
	if err != mtproto.ErrFilePartsInvalid {
		t.Fatalf("video: %v", err)
	}

	prev := profilePhotoID
	profilePhotoID = func(*DfsCore) (int64, error) { return 99, nil }
	t.Cleanup(func() { profilePhotoID = prev })

	got, err := c.DfsUploadProfilePhotoFileV2(&dfs.TLDfsUploadProfilePhotoFileV2{
		Creator:          7,
		VideoEmojiMarkup: emojiMarkup(42, 0x112233, 0x445566),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetId() != 99 || len(got.GetSizes()) != 0 || len(got.GetVideoSizes()) != 1 {
		t.Fatalf("emoji photo %#v", got)
	}
	vs := got.GetVideoSizes()[0]
	if vs.GetPredicateName() != mtproto.Predicate_videoSizeEmojiMarkup || vs.GetEmojiId() != 42 || len(vs.GetBackgroundColors()) != 2 {
		t.Fatalf("markup %#v", vs)
	}
	buf := mtproto.NewEncodeBuf(256)
	if err = got.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}

	_, err = c.DfsUploadProfilePhotoFileV2(&dfs.TLDfsUploadProfilePhotoFileV2{Creator: 7})
	if err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("empty: %v", err)
	}
	_, err = c.DfsUploadProfilePhotoFileV2(&dfs.TLDfsUploadProfilePhotoFileV2{
		Creator:          7,
		VideoEmojiMarkup: emojiMarkup(0, 1),
	})
	if err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("emoji id: %v", err)
	}
	_, err = c.DfsUploadProfilePhotoFileV2(&dfs.TLDfsUploadProfilePhotoFileV2{
		Creator:          7,
		VideoEmojiMarkup: mtproto.MakeTLVideoSize(&mtproto.VideoSize{Type: "v", W: 1, H: 1}).To_VideoSize(),
	})
	if err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("plain video size: %v", err)
	}
}

func TestEmojiMarkupProfilePhotoSticker(t *testing.T) {
	set := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: 5, AccessHash: 6}).To_InputStickerSet()
	photo, err := emojiMarkupProfilePhoto(3, mtproto.MakeTLVideoSizeStickerMarkup(&mtproto.VideoSize{
		Stickerset:       set,
		StickerId:        8,
		BackgroundColors: []int32{1},
	}).To_VideoSize())
	if err != nil {
		t.Fatal(err)
	}
	vs := photo.GetVideoSizes()[0]
	if vs.GetPredicateName() != mtproto.Predicate_videoSizeStickerMarkup || vs.GetStickerId() != 8 || vs.GetStickerset().GetId() != 5 {
		t.Fatalf("sticker %#v", vs)
	}
	if _, err = emojiMarkupProfilePhoto(3, mtproto.MakeTLVideoSizeEmojiMarkup(&mtproto.VideoSize{
		EmojiId:          1,
		BackgroundColors: []int32{1, 2, 3, 4, 5},
	}).To_VideoSize()); err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("colors: %v", err)
	}
}
