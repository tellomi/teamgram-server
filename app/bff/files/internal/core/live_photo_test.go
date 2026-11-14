package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

// ISSUE-S-PHOTO-LIVE-001: messages.uploadMedia is the first step of the iOS
// album path; its messageMediaPhoto must echo the live photo video.
type liveFilesMedia struct {
	mediaclient.MediaClient
}

func (liveFilesMedia) MediaUploadPhotoFile(context.Context, *mediapb.TLMediaUploadPhotoFile) (*mtproto.Photo, error) {
	return mtproto.MakeTLPhoto(&mtproto.Photo{Id: 31, AccessHash: 32, FileReference: []byte{}}).To_Photo(), nil
}

func (liveFilesMedia) MediaGetPhoto(context.Context, *mediapb.TLMediaGetPhoto) (*mtproto.Photo, error) {
	return mtproto.MakeTLPhoto(&mtproto.Photo{Id: 31, AccessHash: 32, FileReference: []byte{}}).To_Photo(), nil
}

func (liveFilesMedia) MediaGetDocument(_ context.Context, in *mediapb.TLMediaGetDocument) (*mtproto.Document, error) {
	if in.GetId() != 41 {
		return nil, mtproto.ErrMediaInvalid
	}
	return mtproto.MakeTLDocument(&mtproto.Document{Id: 41, AccessHash: 42, MimeType: "video/mp4", FileReference: []byte{}}).To_Document(), nil
}

func TestUploadMediaKeepsLivePhotoVideo(t *testing.T) {
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MediaClient: liveFilesMedia{}}})
	c.MD = &metadata.RpcMetadata{UserId: 1, PermAuthKeyId: 11}
	vid := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 41, AccessHash: 42, FileReference: []byte{}}).To_InputDocument()
	file := mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 1, Parts: 1, Name: "a.jpg"}).To_InputFile()
	ph := mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 31, AccessHash: 32, FileReference: []byte{}}).To_InputPhoto()
	for name, in := range map[string]*mtproto.InputMedia{
		"uploadedPhoto": mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{LivePhoto: true, Spoiler: true, File: file, Video: vid}).To_InputMedia(),
		"inputPhoto":    mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{LivePhoto: true, Spoiler: true, Id_INPUTPHOTO: ph, Video: vid}).To_InputMedia(),
	} {
		mm, err := c.makeMediaByInputMedia(in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !mm.GetLivePhoto() || mm.GetVideo_FLAGDOCUMENT().GetId() != 41 || !mm.GetSpoiler() {
			t.Fatalf("%s: live=%v video=%v spoiler=%v", name, mm.GetLivePhoto(), mm.GetVideo_FLAGDOCUMENT(), mm.GetSpoiler())
		}
	}
	plain, err := c.makeMediaByInputMedia(mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{File: file}).To_InputMedia())
	if err != nil || plain.GetLivePhoto() || plain.GetVideo_FLAGDOCUMENT() != nil {
		t.Fatalf("plain photo: %v %v", plain, err)
	}
	bad := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 41, AccessHash: 1, FileReference: []byte{}}).To_InputDocument()
	if _, err = c.makeMediaByInputMedia(mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{LivePhoto: true, File: file, Video: bad}).To_InputMedia()); !errors.Is(err, mtproto.ErrMediaInvalid) {
		t.Fatalf("wrong hash: %v", err)
	}
}
