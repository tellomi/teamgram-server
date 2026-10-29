package crypto

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestOpenEncryptedMessageMTProto2(t *testing.T) {
	authKey := bytesOf(256, 7)
	authKey[200] = 9
	permID := int64(469193)

	// bind_auth_key_inner#75a3f765 nonce:long temp_auth_key_id:long perm_auth_key_id:long temp_session_id:long expires_at:int
	body := make([]byte, 4+8+8+8+8+4)
	binary.LittleEndian.PutUint32(body[0:4], 0x75a3f765)
	binary.LittleEndian.PutUint64(body[4:12], 42)
	binary.LittleEndian.PutUint64(body[12:20], uint64(8412457259831554854))
	binary.LittleEndian.PutUint64(body[20:28], uint64(permID))
	binary.LittleEndian.PutUint64(body[28:36], 7)
	binary.LittleEndian.PutUint32(body[36:40], 1700000000)

	payload := make([]byte, 32+len(body))
	binary.LittleEndian.PutUint64(payload[0:8], 1)
	binary.LittleEndian.PutUint64(payload[8:16], 2)
	binary.LittleEndian.PutUint64(payload[16:24], 4)
	binary.LittleEndian.PutUint32(payload[24:28], 0)
	binary.LittleEndian.PutUint32(payload[28:32], uint32(len(body)))
	copy(payload[32:], body)

	blob, err := SealEncryptedMessage(permID, authKey, payload)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := OpenEncryptedMessage(permID, authKey, blob)
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) < 32+len(body) || !bytes.Equal(plain[32:32+len(body)], body) {
		t.Fatalf("mtproto 2 plaintext mismatch (%d bytes)", len(plain))
	}
}

func bytesOf(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
