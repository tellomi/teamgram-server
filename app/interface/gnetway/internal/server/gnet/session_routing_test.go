package gnet

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func encodeClientMessage(obj mtproto.TLObject) []byte {
	x := mtproto.NewEncodeBuf(256)
	_ = (&mtproto.TLMessage2{
		MsgId:  1 << 32,
		Seqno:  0,
		Object: obj,
	}).Encode(x, 0)
	return append([]byte(nil), x.GetBuf()...)
}

func TestPingFirstUnboundTempKeyUsesItsOwnID(t *testing.T) {
	payload := encodeClientMessage(&mtproto.TLPing{
		Constructor: mtproto.CRC32_ping,
		PingId:      7,
	})

	const tempKey = int64(88001)
	if got := sessionRoutingKey(0, tempKey, payload); got != tempKey {
		t.Fatalf("ping-first session key %d", got)
	}
	if got := sessionRoutingKey(0, tempKey+1, payload); got == tempKey {
		t.Fatal("two unbound temp keys share a session")
	}
	if got := sessionRoutingKey(0, 0, payload); got != 0 {
		t.Fatalf("empty key opened a session %d", got)
	}
}

func TestBindFirstUsesThePermanentID(t *testing.T) {
	const (
		tempKey = int64(88001)
		permKey = int64(469193)
	)
	payload := encodeClientMessage(&mtproto.TLAuthBindTempAuthKey{
		Constructor:      mtproto.CRC32_auth_bindTempAuthKey,
		PermAuthKeyId:    permKey,
		Nonce:            1,
		ExpiresAt:        2,
		EncryptedMessage: []byte{9},
	})

	if got := sessionRoutingKey(0, tempKey, payload); got != permKey {
		t.Fatalf("bind-first session key %d", got)
	}
	// Already bound: a later ping stays on the permanent key.
	ping := encodeClientMessage(&mtproto.TLPing{
		Constructor: mtproto.CRC32_ping,
		PingId:      7,
	})
	if got := sessionRoutingKey(permKey, tempKey, ping); got != permKey {
		t.Fatalf("bound ping session key %d", got)
	}
}
