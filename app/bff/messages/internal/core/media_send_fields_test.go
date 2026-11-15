package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestPaidMediaAndSuggestedPostAreRejected(t *testing.T) {
	c := searchCore(4)
	_, err := c.makeMediaByInputMedia(mtproto.MakeTLInputMediaPaidMedia(&mtproto.InputMedia{
		StarsAmount: 1,
	}).To_InputMedia())
	if status.Convert(err).Message() != "MEDIA_INVALID" {
		t.Fatalf("paid: %v", err)
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer()
	_, err = c.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
		Peer:          peer,
		Message:       "hi",
		SuggestedPost: &mtproto.SuggestedPost{},
	})
	if status.Convert(err).Message() != "SUGGESTED_POST_PEER_INVALID" {
		t.Fatalf("suggested: %v", err)
	}
	_, err = c.sendMedia(&mtproto.TLMessagesSendMedia{
		Peer:          peer,
		SuggestedPost: &mtproto.SuggestedPost{},
	})
	if status.Convert(err).Message() != "SUGGESTED_POST_PEER_INVALID" {
		t.Fatalf("suggested media: %v", err)
	}
}

func TestVideoTimestampAndPhotoStickers(t *testing.T) {
	c := searchCore(4)
	out := mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{}).To_MessageMedia()
	in := mtproto.MakeTLInputMediaDocument(&mtproto.InputMedia{
		VideoTimestamp: wrapperspb.Int32(1),
	}).To_InputMedia()
	if err := applyDocumentExtras(c, out, in); err != nil || out.GetVideoTimestamp().GetValue() != 1 {
		t.Fatalf("timestamp %v %#v", err, out.GetVideoTimestamp())
	}
	missing := mtproto.MakeTLInputMediaDocument(&mtproto.InputMedia{
		VideoCover: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 9}).To_InputPhoto(),
	}).To_InputMedia()
	if err := applyDocumentExtras(c, out, missing); status.Convert(err).Message() != "MEDIA_INVALID" {
		t.Fatalf("cover: %v", err)
	}
	photo := mtproto.MakeTLPhoto(&mtproto.Photo{Id: 3}).To_Photo()
	markPhotoStickers(photo, []*mtproto.InputDocument{{Id: 1}})
	if !photo.GetHasStickers() {
		t.Fatal("stickers flag")
	}
	markPhotoStickers(photo, nil)
	if !photo.GetHasStickers() {
		t.Fatal("flag must stay set")
	}
}
