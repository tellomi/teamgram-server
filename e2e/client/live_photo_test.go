package e2e

import (
	"bytes"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestLivePhoto is ISSUE-S-PHOTO-LIVE-001: the official clients send a live
// photo as a photo plus a separately uploaded video document (layer 232
// inputMediaUploadedPhoto / inputMediaPhoto video). Both the single send and
// the iOS album path (uploadMedia, then sendMultiMedia with inputMediaPhoto)
// must deliver messageMediaPhoto{live_photo, video} to sender and recipient.
func TestLivePhoto(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "live_a", "LiveA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "live_b", "LiveB")
	peerOf := func(from *Client, name, first string) *mtproto.InputPeer {
		t.Helper()
		ph, err := cfg.Phone(name)
		if err != nil {
			t.Fatal(err)
		}
		cx, cancel := ctx()
		defer cancel()
		id, hash, err := from.ImportPhone(cx, ph.Number, first)
		if err != nil {
			t.Fatal(err)
		}
		return inputPeerUser(id, hash)
	}
	aToB := peerOf(a, "live_b", "LiveB")
	bToA := peerOf(b, "live_a", "LiveA")

	// 1) the video half, uploaded the way both clients do it.
	payload := bytes.Repeat([]byte("tellomi-live-photo-video\n"), 300)
	vm := invokeOK(t, a, &mtproto.TLMessagesUploadMedia{
		Constructor: mtproto.CRC32_messages_uploadMedia_14967978,
		Peer:        aToB,
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File:         mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: uploadBytes(t, a, payload), Parts: 1, Name: "live.mp4"}).To_InputFile(),
			MimeType:     "video/mp4",
			NosoundVideo: true,
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{Duration: 2, W: 160, H: 120}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
	})
	vd, ok := vm.(*mtproto.TLMessageMediaDocument)
	if !ok || vd.GetDocument().GetId() == 0 {
		t.Fatalf("uploadMedia(video) returned %T", vm)
	}
	video := vd.GetDocument()
	inVideo := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: video.GetId(), AccessHash: video.GetAccessHash(), FileReference: video.GetFileReference()}).To_InputDocument()

	isLive := func(m *mtproto.MessageMedia) bool {
		return m.GetPredicateName() == mtproto.Predicate_messageMediaPhoto && m.GetLivePhoto() && m.GetVideo_FLAGDOCUMENT().GetId() == video.GetId()
	}

	// 2) single send.
	res := invokeOK(t, a, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        aToB,
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			LivePhoto: true,
			File:      mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: uploadBytes(t, a, testJPEG(t, 320, 240, 5)), Parts: 1, Name: "live.jpg"}).To_InputFile(),
			Video:     inVideo,
		}).To_InputMedia(),
		Message:  "live-single",
		RandomId: randomID(),
	})
	sent := 0
	for _, m := range allMessages(res) {
		if m.GetMedia() != nil {
			sent++
			if !isLive(m.GetMedia()) {
				t.Fatalf("sendMedia echo lost the live video: %v", m.GetMedia())
			}
		}
	}
	if sent == 0 {
		t.Fatalf("sendMedia returned no message (%T)", res)
	}

	// 3) iOS album path.
	um := invokeOK(t, a, &mtproto.TLMessagesUploadMedia{
		Constructor: mtproto.CRC32_messages_uploadMedia_14967978,
		Peer:        aToB,
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			LivePhoto: true,
			File:      mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: uploadBytes(t, a, testJPEG(t, 320, 240, 6)), Parts: 1, Name: "live2.jpg"}).To_InputFile(),
			Video:     inVideo,
		}).To_InputMedia(),
	})
	up, ok := um.(*mtproto.TLMessageMediaPhoto)
	if !ok || !isLive(up.To_MessageMedia()) {
		t.Fatalf("uploadMedia(live photo) echo %T %v", um, um)
	}
	p := up.GetPhoto_FLAGPHOTO()
	inPhoto := mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: p.GetId(), AccessHash: p.GetAccessHash(), FileReference: p.GetFileReference()}).To_InputPhoto()
	var multi []*mtproto.InputSingleMedia
	for i := 0; i < 2; i++ {
		multi = append(multi, mtproto.MakeTLInputSingleMedia(&mtproto.InputSingleMedia{
			Media:    mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{LivePhoto: true, Id_INPUTPHOTO: inPhoto, Video: inVideo}).To_InputMedia(),
			RandomId: randomID(),
			Message:  "live-album",
		}).To_InputSingleMedia())
	}
	invokeOK(t, a, &mtproto.TLMessagesSendMultiMedia{Constructor: mtproto.CRC32_messages_sendMultiMedia_1bf89d74, Peer: aToB, MultiMedia: multi})

	// 4) both sides read it back from history.
	want := map[string]int{"live-single": 1, "live-album": 2}
	for name, c := range map[string]struct {
		cl   *Client
		peer *mtproto.InputPeer
	}{"sender": {a, aToB}, "recipient": {b, bToA}} {
		deadline := time.Now().Add(20 * time.Second)
		for {
			got := map[string]int{}
			hist := invokeOK(t, c.cl, &mtproto.TLMessagesGetHistory{Constructor: mtproto.CRC32_messages_getHistory, Peer: c.peer, Limit: 20})
			for _, m := range allMessages(hist) {
				if isLive(m.GetMedia()) {
					got[m.GetMessage()]++
				}
			}
			if got["live-single"] == want["live-single"] && got["live-album"] == want["live-album"] {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s history live photos %v, want %v", name, got, want)
			}
			time.Sleep(300 * time.Millisecond)
		}
	}

	// 5) a video the sender does not hold is rejected, not silently dropped.
	expectRPC(t, a, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        aToB,
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			LivePhoto: true,
			File:      mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: uploadBytes(t, a, testJPEG(t, 320, 240, 7)), Parts: 1, Name: "live3.jpg"}).To_InputFile(),
			Video:     mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: video.GetId(), AccessHash: video.GetAccessHash() + 1, FileReference: video.GetFileReference()}).To_InputDocument(),
		}).To_InputMedia(),
		RandomId: randomID(),
	}, "MEDIA_INVALID")
}
