package secretchat

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDiscardAllDropsHistoryAndIsRetryable(t *testing.T) {
	Reset()
	s := Default()
	waiting, _, err := s.Request(1, 2, 42, []byte{2}, 10)
	if err != nil {
		t.Fatal(err)
	}
	peer := mtproto.MakeTLInputEncryptedChat(&mtproto.InputEncryptedChat{
		ChatId: waiting.GetId(), AccessHash: waiting.GetAccessHash(),
	}).To_InputEncryptedChat()
	if _, _, err = s.Accept(2, peer, []byte{3}, 1, 11); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Send(1, peer, 9, []byte("cipher"), mtproto.MakeTLEncryptedFile(&mtproto.EncryptedFile{Id: 99}).To_EncryptedFile(), false, 12); err != nil {
		t.Fatal(err)
	}
	events, files := s.DiscardAll(1, 20)
	if len(events) != 1 || events[0].UserID != 2 || events[0].Update.GetChat().GetHistoryDeleted() != true {
		t.Fatalf("events = %+v", events)
	}
	if len(files) != 1 || files[0] != 99 {
		t.Fatalf("files = %v", files)
	}
	if again, more := s.DiscardAll(1, 21); len(again) != 0 || len(more) != 0 {
		t.Fatalf("retry events=%d files=%d", len(again), len(more))
	}
	if _, err := s.Discard(1, 42, true, 22); err != mtproto.ErrEncryptionAlreadyDeclined {
		t.Fatalf("discard after account deletion: %v", err)
	}
}
