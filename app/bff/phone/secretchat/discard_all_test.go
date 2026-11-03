package secretchat

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDiscardAllDropsHistoryAndIsRetryable(t *testing.T) {
	Reset()
	s := Default()
	if _, _, err := s.Request(1, 2, 42, []byte{2}, 10); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.chats[42].phase = phaseAccepted
	s.sent[sentKey{chat: 42, random: 9}] = sentRec{
		file: mtproto.MakeTLEncryptedFile(&mtproto.EncryptedFile{Id: 99}).To_EncryptedFile(),
	}
	s.mu.Unlock()
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
