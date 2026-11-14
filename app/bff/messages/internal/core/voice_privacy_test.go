package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestInputMediaIsVoice(t *testing.T) {
	voice := mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
		Attributes: []*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: true}).To_DocumentAttribute(),
		},
	}).To_InputMedia()
	if !inputMediaIsVoice(voice) {
		t.Fatal("voice note")
	}
	song := mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
		Attributes: []*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: false}).To_DocumentAttribute(),
		},
	}).To_InputMedia()
	if inputMediaIsVoice(song) {
		t.Fatal("music is not a voice message")
	}
	photo := mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{}).To_InputMedia()
	if inputMediaIsVoice(photo) {
		t.Fatal("photo")
	}
}
