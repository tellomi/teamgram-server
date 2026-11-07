package secretchat

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/msgexpiry"
)

func TestSecretDownloadAndDifferenceStayExpired(t *testing.T) {
	t.Setenv(msgexpiry.EnvEnabled, "1")
	t.Setenv(msgexpiry.EnvSweep, "1")
	be := newMem()
	admin := New(be)
	waiting, _, err := admin.Request(1, 2, 7, []byte{1}, 10)
	if err != nil {
		t.Fatal(err)
	}
	peer := New(be)
	if _, _, err = peer.Accept(2, input(waiting), []byte{2}, 3, 11); err != nil {
		t.Fatal(err)
	}
	in := input(waiting)
	if err = admin.SetTTL(1, in, 5); err != nil {
		t.Fatal(err)
	}
	file := mtproto.MakeTLEncryptedFile(&mtproto.EncryptedFile{Id: 99, AccessHash: 1}).To_EncryptedFile()
	admin.RememberEncrypted(waiting.GetId(), file)
	sentAt := int32(time.Now().Unix()) - 30
	if _, _, err = admin.Send(1, in, 50, []byte("cipher"), file, false, sentAt); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := admin.LookupEncrypted(99); ok {
		t.Fatal("expired secret file still downloads")
	}
	diff := ApplyDifference(2, 0, 20, nil)
	for _, m := range diff.GetNewEncryptedMessages() {
		if m.GetRandomId() == 50 && len(m.GetBytes()) > 0 {
			t.Fatal("difference resurrected ciphertext")
		}
	}
	for _, u := range diff.GetOtherUpdates() {
		if em := u.GetMessage_ENCRYPTEDMESSAGE(); em != nil && em.GetRandomId() == 50 {
			t.Fatal("other update resurrected ciphertext")
		}
	}
	first := msgexpiry.Default().Sweep(int32(time.Now().Unix()))
	if len(first) == 0 {
		t.Fatal("sweep did not delete the secret record")
	}
	if again := msgexpiry.Default().Sweep(int32(time.Now().Unix())); len(again) != 0 {
		t.Fatal("sweep was not idempotent")
	}
}
