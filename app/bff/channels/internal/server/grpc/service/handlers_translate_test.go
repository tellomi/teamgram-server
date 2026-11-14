package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestTranslateTextWithoutPeerOrText(t *testing.T) {
	_, err := (TranslationService{}).MessagesTranslateText(context.Background(), &mtproto.TLMessagesTranslateText{
		ToLang: "zh",
	})
	if err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("got %v", err)
	}
}
