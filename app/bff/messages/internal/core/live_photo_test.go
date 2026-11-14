package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/zeromicro/go-zero/core/logx"
)

// ISSUE-S-PHOTO-LIVE-001: sendMedia / sendMultiMedia / editMessage must keep
// the live photo video the official clients send (layer 232).
type liveMedia struct {
	mediaclient.MediaClient
	photo *mtproto.Photo
	docs  map[int64]*mtproto.Document
}

func (m *liveMedia) MediaUploadPhotoFile(context.Context, *mediapb.TLMediaUploadPhotoFile) (*mtproto.Photo, error) {
	return m.photo, nil
}

func (m *liveMedia) MediaGetPhoto(_ context.Context, in *mediapb.TLMediaGetPhoto) (*mtproto.Photo, error) {
	if in.GetPhotoId() != m.photo.GetId() {
		return nil, mtproto.ErrMediaInvalid
	}
	return m.photo, nil
}

func (m *liveMedia) MediaGetDocument(_ context.Context, in *mediapb.TLMediaGetDocument) (*mtproto.Document, error) {
	if d, ok := m.docs[in.GetId()]; ok {
		return d, nil
	}
	return nil, mtproto.ErrMediaInvalid
}

func newLiveFixture() (*liveMedia, *mtproto.InputDocument, *mtproto.InputPhoto) {
	m := &liveMedia{
		photo: mtproto.MakeTLPhoto(&mtproto.Photo{Id: 9001, AccessHash: 91, FileReference: []byte{}, Date: 1, DcId: 1}).To_Photo(),
		docs: map[int64]*mtproto.Document{
			9101: mtproto.MakeTLDocument(&mtproto.Document{Id: 9101, AccessHash: 92, MimeType: "video/mp4", FileReference: []byte{}}).To_Document(),
			9102: mtproto.MakeTLDocument(&mtproto.Document{Id: 9102, AccessHash: 93, MimeType: "image/png", FileReference: []byte{}}).To_Document(),
		},
	}
	vid := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 9101, AccessHash: 92, FileReference: []byte{}}).To_InputDocument()
	ph := mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 9001, AccessHash: 91, FileReference: []byte{}}).To_InputPhoto()
	return m, vid, ph
}

func liveCore(m *liveMedia) *MessagesCore {
	return &MessagesCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MediaClient: m}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1, PermAuthKeyId: 11},
	}
}

func TestMakeMediaKeepsLivePhotoVideo(t *testing.T) {
	m, vid, ph := newLiveFixture()
	c := liveCore(m)
	cases := map[string]*mtproto.InputMedia{
		"uploadedPhoto": mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{LivePhoto: true, File: mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 1, Parts: 1, Name: "a.jpg"}).To_InputFile(), Video: vid}).To_InputMedia(),
		"inputPhoto":    mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{LivePhoto: true, Id_INPUTPHOTO: ph, Video: vid}).To_InputMedia(),
	}
	for name, in := range cases {
		if err := c.authorizeReferencedMedia(in); err != nil {
			t.Fatalf("%s authorize: %v", name, err)
		}
		mm, err := c.makeMediaByInputMedia(in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !mm.GetLivePhoto() || mm.GetVideo_FLAGDOCUMENT().GetId() != 9101 || mm.GetPhoto_FLAGPHOTO().GetId() != 9001 {
			t.Fatalf("%s: live=%v video=%v", name, mm.GetLivePhoto(), mm.GetVideo_FLAGDOCUMENT())
		}
	}
}

func TestMakeMediaPlainPhotoHasNoVideo(t *testing.T) {
	m, _, ph := newLiveFixture()
	mm, err := liveCore(m).makeMediaByInputMedia(mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{Id_INPUTPHOTO: ph}).To_InputMedia())
	if err != nil {
		t.Fatal(err)
	}
	if mm.GetLivePhoto() || mm.GetVideo_FLAGDOCUMENT() != nil {
		t.Fatalf("plain photo became live: %v", mm)
	}
}

func TestMakeMediaRejectsBadLiveVideo(t *testing.T) {
	m, vid, ph := newLiveFixture()
	c := liveCore(m)
	wrongHash := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: vid.GetId(), AccessHash: vid.GetAccessHash() + 1, FileReference: []byte{}}).To_InputDocument()
	notVideo := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 9102, AccessHash: 93, FileReference: []byte{}}).To_InputDocument()
	missing := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 9199, AccessHash: 1, FileReference: []byte{}}).To_InputDocument()
	for name, v := range map[string]*mtproto.InputDocument{"wrongHash": wrongHash, "notVideo": notVideo, "missing": missing} {
		for kind, in := range map[string]*mtproto.InputMedia{
			"uploadedPhoto": mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{LivePhoto: true, File: mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 1, Parts: 1, Name: "a.jpg"}).To_InputFile(), Video: v}).To_InputMedia(),
			"inputPhoto":    mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{LivePhoto: true, Id_INPUTPHOTO: ph, Video: v}).To_InputMedia(),
		} {
			if _, err := c.makeMediaByInputMedia(in); !errors.Is(err, mtproto.ErrMediaInvalid) {
				t.Fatalf("%s/%s make: %v", name, kind, err)
			}
			if err := c.authorizeReferencedMedia(in); !errors.Is(err, mtproto.ErrMediaInvalid) {
				t.Fatalf("%s/%s authorize: %v", name, kind, err)
			}
		}
	}
}
