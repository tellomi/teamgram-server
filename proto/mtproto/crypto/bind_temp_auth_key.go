package crypto

import "fmt"

// OpenEncryptedMessage decrypts an MTProto message blob
// (auth_key_id:int64 + msg_key:int128 + encrypted_data).
// iOS speaks MTProto 2.0 (SHA256). Older clients still use the SHA1 msg_key,
// so that layout is tried when the 2.0 check fails.
func OpenEncryptedMessage(authKeyID int64, authKey, encrypted []byte) ([]byte, error) {
	if len(encrypted) < 8+16+16 {
		return nil, fmt.Errorf("encrypted message too short: %d", len(encrypted))
	}
	key := NewAuthKey(authKeyID, authKey)
	msgKey := encrypted[8 : 8+16]
	body := encrypted[8+16:]
	if plain, err := key.AesIgeDecrypt(msgKey, body); err == nil {
		return plain, nil
	}
	return key.AesIgeDecryptV1(msgKey, body)
}

// SealEncryptedMessage builds the same blob OpenEncryptedMessage reads,
// using MTProto 2.0. plain is the decrypted payload (salt through padding).
func SealEncryptedMessage(authKeyID int64, authKey, plain []byte) ([]byte, error) {
	key := NewAuthKey(authKeyID, authKey)
	msgKey, body, err := key.AesIgeEncrypt(plain)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 8+len(msgKey)+len(body))
	var id [8]byte
	// auth_key_id is little-endian on the wire; the decrypt path does not
	// check it (the caller already selected the key). Keep the bytes stable.
	id[0] = byte(authKeyID)
	id[1] = byte(authKeyID >> 8)
	id[2] = byte(authKeyID >> 16)
	id[3] = byte(authKeyID >> 24)
	id[4] = byte(authKeyID >> 32)
	id[5] = byte(authKeyID >> 40)
	id[6] = byte(authKeyID >> 48)
	id[7] = byte(authKeyID >> 56)
	out = append(out, id[:]...)
	out = append(out, msgKey...)
	out = append(out, body...)
	return out, nil
}
