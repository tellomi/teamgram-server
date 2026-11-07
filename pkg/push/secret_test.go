package push

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSecretChatPushWakesWithoutCiphertext(t *testing.T) {
	const cipher = "cipher-text-not-for-the-banner"
	invite := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateEncryption(&mtproto.Update{
		Chat: mtproto.MakeTLEncryptedChatRequested(&mtproto.EncryptedChat{
			Id: 42, AdminId: 7, ParticipantId: 9,
		}).To_EncryptedChat(),
	}).To_Update())
	if !ShouldDeliver(9, invite) {
		t.Fatal("an encrypted chat request did not qualify for push")
	}
	got := Messages(9, invite)
	if len(got) != 1 || got[0].LocKey != "ENCRYPTED_MESSAGE" || got[0].SenderID != 7 || got[0].Text != "" {
		t.Fatalf("invite push %+v", got)
	}

	msg := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNewEncryptedMessage(&mtproto.Update{
		Qts: 3,
		Message_ENCRYPTEDMESSAGE: mtproto.MakeTLEncryptedMessage(&mtproto.EncryptedMessage{
			ChatId: 42, RandomId: 100, Bytes: []byte(cipher),
		}).To_EncryptedMessage(),
	}).To_Update())
	if !ShouldDeliver(9, msg) {
		t.Fatal("an encrypted message did not qualify for push")
	}
	got = Messages(9, msg)
	if len(got) != 1 || got[0].LocKey != "ENCRYPTED_MESSAGE" || got[0].Text != "" {
		t.Fatalf("message push %+v", got)
	}
	alert, err := BuildAlert(got[0], Decision{Send: true, Preview: true, Sound: "default"}, nil, "en")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(alert.Outer), cipher) || strings.Contains(string(alert.Android), cipher) {
		t.Fatal("push payload included the secret-chat ciphertext")
	}
	if !strings.Contains(string(alert.Outer), "ENCRYPTED_MESSAGE") {
		t.Fatalf("loc-key missing: %s", alert.Outer)
	}
}
