package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestRSAPadRoundTripKeepsLeadingZeroPlaintext(t *testing.T) {
	key, err := NewRSACryptorByKeyData(pkcs1PemPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	// Constructor p_q_inner_data_temp_dc#56fddf88 plus a few payload bytes.
	plain := []byte{0x88, 0xdf, 0xfd, 0x56, 0x04, 0x17, 0xed, 0x48, 0x94}

	var encrypted []byte
	sawShort := false
	for i := 0; i < 12; i++ {
		enc, err := EncryptRSAPad(&key.key.PublicKey, plain, rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if len(enc) != 256 {
			t.Fatalf("ciphertext len %d", len(enc))
		}
		if len(key.Decrypt(enc)) < 256 {
			sawShort = true
		}
		encrypted = enc
		got, err := DecryptPQInner(key, enc)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(got, plain) {
			t.Fatalf("plaintext prefix lost: %x", got[:len(plain)])
		}
	}
	if len(key.DecryptBlock(encrypted)) != 256 {
		t.Fatal("DecryptBlock must keep 256 bytes when big.Int drops a leading zero")
	}
	if !sawShort {
		t.Log("no leading-zero RSA plaintext in 12 tries; padding is still applied")
	}
}

func TestRSAPadRejectsShortCiphertext(t *testing.T) {
	key, err := NewRSACryptorByKeyData(pkcs1PemPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptPQInner(key, make([]byte, 255)); err == nil {
		t.Fatal("expected length error")
	}
}
