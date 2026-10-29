package push

import (
	"bytes"
	"crypto/sha1"
	"testing"
)

func TestNotificationRoundTripMatchesIOSLayout(t *testing.T) {
	secret := bytes.Repeat([]byte{0x11}, 256)
	payload := []byte(`{"msg_id":"7","from_id":"3","aps":{"alert":{"title":"Ada","body":"hi"}}}`)
	encoded, err := EncryptNotification(secret, payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := decodeP(encoded)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum(secret)
	if !bytes.Equal(raw[:8], sum[len(sum)-8:]) {
		t.Fatalf("key id = %x, want last 8 of sha1 %x", raw[:8], sum[len(sum)-8:])
	}
	if len(raw) < 8+16+16 || (len(raw)-(8+16))%16 != 0 {
		t.Fatalf("blob len %d", len(raw))
	}
	got, err := DecryptNotification(secret, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %s", got)
	}
	if _, err := DecryptNotification(bytes.Repeat([]byte{0x22}, 256), encoded); err == nil {
		t.Fatal("wrong secret decrypted")
	}
}

func TestNotificationRoundTripVariousLengths(t *testing.T) {
	secret := bytes.Repeat([]byte{0x7e}, 256)
	for _, n := range []int{0, 1, 12, 13, 15, 16, 31, 64, 200} {
		payload := bytes.Repeat([]byte("x"), n)
		encoded, err := EncryptNotification(secret, payload)
		if err != nil {
			t.Fatal(n, err)
		}
		got, err := DecryptNotification(secret, encoded)
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("len %d: %v %q", n, err, got)
		}
	}
}

func TestDecryptRejectsShortSecret(t *testing.T) {
	if _, err := EncryptNotification([]byte("short"), []byte("{}")); err == nil {
		t.Fatal("expected error")
	}
}
