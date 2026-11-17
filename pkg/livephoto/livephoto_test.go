package livephoto

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func videoDoc(id int64, mime string) *mtproto.Document {
	return mtproto.MakeTLDocument(&mtproto.Document{Id: id, AccessHash: id + 1, MimeType: mime, FileReference: []byte{}}).To_Document()
}

func inDoc(id int64) *mtproto.InputDocument {
	return mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: id, AccessHash: id + 1, FileReference: []byte{}}).To_InputDocument()
}

func TestVideo(t *testing.T) {
	store := map[int64]*mtproto.Document{7: videoDoc(7, "video/mp4"), 8: videoDoc(8, "image/jpeg")}
	resolve := func(in *mtproto.InputDocument) (*mtproto.Document, error) {
		d, ok := store[in.GetId()]
		if !ok || d.GetAccessHash() != in.GetAccessHash() {
			return nil, mtproto.ErrMediaInvalid
		}
		return d, nil
	}
	plain := mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{}).To_InputMedia()
	if d, err := Video(plain, resolve); d != nil || err != nil {
		t.Fatalf("plain photo: %v %v", d, err)
	}
	empty := mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{Video: mtproto.MakeTLInputDocumentEmpty(nil).To_InputDocument()}).To_InputMedia()
	if d, err := Video(empty, resolve); d != nil || err != nil {
		t.Fatalf("inputDocumentEmpty: %v %v", d, err)
	}
	live := mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{LivePhoto: true, Video: inDoc(7)}).To_InputMedia()
	if d, err := Video(live, resolve); err != nil || d.GetId() != 7 {
		t.Fatalf("live: %v %v", d, err)
	}
	bad := inDoc(7)
	bad.AccessHash++
	if _, err := Video(mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{LivePhoto: true, Video: bad}).To_InputMedia(), resolve); !errors.Is(err, mtproto.ErrMediaInvalid) {
		t.Fatalf("wrong hash: %v", err)
	}
	if _, err := Video(mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{LivePhoto: true, Video: inDoc(8)}).To_InputMedia(), resolve); !errors.Is(err, mtproto.ErrMediaInvalid) {
		t.Fatalf("not a video: %v", err)
	}
	if _, err := Video(live, nil); !errors.Is(err, mtproto.ErrMediaInvalid) {
		t.Fatalf("no resolver: %v", err)
	}
}

func TestIsVideoByAttribute(t *testing.T) {
	d := videoDoc(9, "application/octet-stream")
	if IsVideo(d) {
		t.Fatal("octet-stream without attribute is not a video")
	}
	d.Attributes = []*mtproto.DocumentAttribute{mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{W: 1, H: 1}).To_DocumentAttribute()}
	if !IsVideo(d) {
		t.Fatal("documentAttributeVideo is a video")
	}
}

func TestSetKeepsFlagAndVideoTogether(t *testing.T) {
	mm := mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{LivePhoto: true}).To_MessageMedia()
	Set(mm, nil)
	if mm.GetLivePhoto() || mm.GetVideo_FLAGDOCUMENT() != nil {
		t.Fatal("no video must clear live_photo")
	}
	Set(mm, videoDoc(7, "video/mp4"))
	if !mm.GetLivePhoto() || mm.GetVideo_FLAGDOCUMENT().GetId() != 7 {
		t.Fatal("video must set live_photo")
	}
}

// The wire form a layer 232 client reads back: flags.4 and the Document are
// both present, and decoding yields the same video id.
func TestEncodeRoundTrip(t *testing.T) {
	photo := mtproto.MakeTLPhoto(&mtproto.Photo{Id: 3, AccessHash: 4, FileReference: []byte{}, Date: 1, DcId: 1}).To_Photo()
	mm := Set(mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{Photo_FLAGPHOTO: photo}).To_MessageMedia(), videoDoc(7, "video/mp4"))
	x := mtproto.NewEncodeBuf(512)
	if err := mm.Encode(x, 232); err != nil {
		t.Fatal(err)
	}
	d := mtproto.NewDecodeBuf(x.GetBuf())
	got := &mtproto.MessageMedia{}
	if err := got.Decode(d); err != nil {
		t.Fatal(err)
	}
	if !got.GetLivePhoto() || got.GetVideo_FLAGDOCUMENT().GetId() != 7 || got.GetPhoto_FLAGPHOTO().GetId() != 3 {
		t.Fatalf("round trip: live=%v video=%v photo=%v", got.GetLivePhoto(), got.GetVideo_FLAGDOCUMENT(), got.GetPhoto_FLAGPHOTO())
	}
}

// The request a layer 232 client sends: live_photo and video share flags.3
// on inputMediaUploadedPhoto and flags.2 on inputMediaPhoto.
func TestInputMediaRoundTrip(t *testing.T) {
	for name, in := range map[string]*mtproto.InputMedia{
		"uploadedPhoto": mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{LivePhoto: true, File: mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 1, Parts: 1, Name: "a.jpg"}).To_InputFile(), Video: inDoc(7)}).To_InputMedia(),
		"inputPhoto":    mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{LivePhoto: true, Id_INPUTPHOTO: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 3, AccessHash: 4, FileReference: []byte{}}).To_InputPhoto(), Video: inDoc(7)}).To_InputMedia(),
	} {
		x := mtproto.NewEncodeBuf(256)
		if err := in.Encode(x, 232); err != nil {
			t.Fatal(name, err)
		}
		got := &mtproto.InputMedia{}
		if err := got.Decode(mtproto.NewDecodeBuf(x.GetBuf())); err != nil {
			t.Fatal(name, err)
		}
		if !got.GetLivePhoto() || got.GetVideo().GetId() != 7 {
			t.Fatalf("%s: live=%v video=%v", name, got.GetLivePhoto(), got.GetVideo())
		}
	}
}
