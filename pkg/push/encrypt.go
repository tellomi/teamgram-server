package push

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"

	"github.com/teamgram/proto/mtproto/crypto"
)

// EncryptNotification seals JSON for the Telegram-iOS NotificationService.
//
// The client (Account.swift, decryptedNotificationPayload) expects:
//
//	key_id (8 bytes, last 8 of SHA1(secret)) || msg_key (16) || AES-IGE ciphertext
//
// Encryption is MTProto 2.0 with x = 8 (server to client). The plaintext is
// a 32-bit little-endian JSON length, the JSON, then 12..1024 bytes of padding
// so the length is a multiple of 16. The result is unpadded base64url, which
// the extension accepts in the "p" field.
func EncryptNotification(secret, payload []byte) (string, error) {
	raw, err := sealNotification(secret, payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecryptNotification reverses EncryptNotification. It follows the same checks
// as Telegram-iOS decryptedNotificationPayload.
func DecryptNotification(secret []byte, encoded string) ([]byte, error) {
	raw, err := decodeP(encoded)
	if err != nil {
		return nil, err
	}
	return openNotification(secret, raw)
}

func sealNotification(secret, payload []byte) ([]byte, error) {
	if len(secret) < 128 {
		return nil, fmt.Errorf("push secret must be at least 128 bytes, got %d", len(secret))
	}
	if len(payload) > 1<<20 {
		return nil, fmt.Errorf("push payload too large")
	}
	plain := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(plain[:4], uint32(len(payload)))
	copy(plain[4:], payload)

	key := crypto.NewAuthKey(0, secret)
	msgKey, body, err := key.AesIgeEncrypt(plain)
	if err != nil {
		return nil, err
	}
	var id [8]byte
	binary.LittleEndian.PutUint64(id[:], uint64(key.AuthKeyId()))
	out := make([]byte, 0, 8+len(msgKey)+len(body))
	out = append(out, id[:]...)
	out = append(out, msgKey...)
	out = append(out, body...)
	return out, nil
}

func openNotification(secret, data []byte) ([]byte, error) {
	if len(secret) < 128 {
		return nil, fmt.Errorf("push secret must be at least 128 bytes, got %d", len(secret))
	}
	if len(data) < 8+16+16 {
		return nil, fmt.Errorf("encrypted push too short")
	}
	sum := sha1.Sum(secret)
	keyID := sum[len(sum)-8:]
	if string(data[:8]) != string(keyID) {
		return nil, fmt.Errorf("notification key id mismatch")
	}
	const x = 8
	msgKey := data[8 : 8+16]
	raw := data[8+16:]
	if len(raw)%16 != 0 {
		return nil, fmt.Errorf("encrypted push is not block aligned")
	}

	shaA := sha256.Sum256(append(append([]byte{}, msgKey...), secret[x:x+36]...))
	shaB := sha256.Sum256(append(append([]byte{}, secret[40+x:40+x+36]...), msgKey...))
	aesKey := make([]byte, 32)
	aesIV := make([]byte, 32)
	copy(aesKey[:8], shaA[:8])
	copy(aesKey[8:24], shaB[8:24])
	copy(aesKey[24:], shaA[24:32])
	copy(aesIV[:8], shaB[:8])
	copy(aesIV[8:24], shaA[8:24])
	copy(aesIV[24:], shaB[24:32])

	c := crypto.NewAES256IGECryptor(aesKey, aesIV)
	if c == nil {
		return nil, fmt.Errorf("aes-ige init failed")
	}
	plain, err := c.Decrypt(raw)
	if err != nil {
		return nil, err
	}
	if len(plain) < 4 {
		return nil, fmt.Errorf("decrypted push too short")
	}
	check := sha256.Sum256(append(append([]byte{}, secret[88+x:88+x+32]...), plain...))
	if string(check[8:24]) != string(msgKey) {
		return nil, fmt.Errorf("notification msg_key mismatch")
	}
	n := int(binary.LittleEndian.Uint32(plain[:4]))
	if n < 0 || n > len(plain)-4 {
		return nil, fmt.Errorf("notification length out of range")
	}
	pad := len(plain) - 4 - n
	if pad < 12 || pad > 1024 {
		return nil, fmt.Errorf("notification padding %d out of range", pad)
	}
	return append([]byte(nil), plain[4:4+n]...), nil
}

func decodeP(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, fmt.Errorf("empty push payload")
	}
	// The extension rewrites '-' and '_' back to standard base64 and adds padding.
	if raw, err := base64.RawURLEncoding.DecodeString(encoded); err == nil {
		return raw, nil
	}
	if raw, err := base64.URLEncoding.DecodeString(encoded); err == nil {
		return raw, nil
	}
	return base64.StdEncoding.DecodeString(encoded)
}
