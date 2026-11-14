package dao

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	media_client "github.com/teamgram/teamgram-server/app/service/media/client"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

type liveHelper struct {
	user_client.UserClient
	media_client.MediaClient
}

func (liveHelper) MediaGetPhoto(context.Context, *mediapb.TLMediaGetPhoto) (*mtproto.Photo, error) {
	return mtproto.MakeTLPhoto(&mtproto.Photo{Id: 5, AccessHash: 6, FileReference: []byte{}}).To_Photo(), nil
}

func (liveHelper) MediaGetDocument(_ context.Context, in *mediapb.TLMediaGetDocument) (*mtproto.Document, error) {
	if in.GetId() != 7 {
		return nil, mtproto.ErrMediaInvalid
	}
	return mtproto.MakeTLDocument(&mtproto.Document{Id: 7, AccessHash: 8, MimeType: "video/mp4", FileReference: []byte{}}).To_Document(), nil
}

func TestGetMessageMediaKeepsLivePhotoVideo(t *testing.T) {
	in := mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{
		LivePhoto:     true,
		Id_INPUTPHOTO: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 5, AccessHash: 6, FileReference: []byte{}}).To_InputPhoto(),
		Video:         mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 7, AccessHash: 8, FileReference: []byte{}}).To_InputDocument(),
	}).To_InputMedia()
	mm, err := GetMessageMedia(context.Background(), liveHelper{}, 1, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	if !mm.GetLivePhoto() || mm.GetVideo_FLAGDOCUMENT().GetId() != 7 {
		t.Fatalf("live=%v video=%v", mm.GetLivePhoto(), mm.GetVideo_FLAGDOCUMENT())
	}
}
