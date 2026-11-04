package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/media"
)

func TestDfsProfilePhotoRequest(t *testing.T) {
	file := mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 1, Parts: 1, Name: "a.jpg"}).To_InputFile()
	video := mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 2, Parts: 1, Name: "a.mp4"}).To_InputFile()
	markup := mtproto.MakeTLVideoSizeEmojiMarkup(&mtproto.VideoSize{
		EmojiId:          9,
		BackgroundColors: []int32{1, 2},
	}).To_VideoSize()

	photoOnly, err := dfsProfilePhotoRequest(&media.TLMediaUploadProfilePhotoFile{OwnerId: 4, File: file})
	if err != nil || photoOnly.GetFile() != file || photoOnly.GetVideo() != nil || photoOnly.GetCreator() != 4 {
		t.Fatalf("photo %#v %v", photoOnly, err)
	}

	videoOnly, err := dfsProfilePhotoRequest(&media.TLMediaUploadProfilePhotoFile{OwnerId: 4, Video: video})
	if err != nil || videoOnly.GetVideo() != video || videoOnly.GetFile() != nil {
		t.Fatalf("video %#v %v", videoOnly, err)
	}

	emojiOnly, err := dfsProfilePhotoRequest(&media.TLMediaUploadProfilePhotoFile{OwnerId: 4, VideoEmojiMarkup: markup})
	if err != nil || emojiOnly.GetVideoEmojiMarkup() != markup || emojiOnly.GetFile() != nil || emojiOnly.GetVideo() != nil {
		t.Fatalf("emoji %#v %v", emojiOnly, err)
	}

	if _, err = dfsProfilePhotoRequest(&media.TLMediaUploadProfilePhotoFile{OwnerId: 4}); err != mtproto.ErrMediaInvalid {
		t.Fatalf("empty: %v", err)
	}
	if _, err = dfsProfilePhotoRequest(nil); err != mtproto.ErrMediaInvalid {
		t.Fatalf("nil: %v", err)
	}
}
