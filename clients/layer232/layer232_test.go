package layer232

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// The server module resolves github.com/teamgram/proto to ./proto (see go.mod).
// This checks that a layer-232 invokeWithLayer no longer encodes constructor 0,
// and that the constructors the register/login/private-message flow touches
// match the stock client schema.
func TestServerSpeaksLayer232(t *testing.T) {
	if mtproto.Layer != 232 {
		t.Fatalf("mtproto.Layer = %d", mtproto.Layer)
	}
	checks := []struct {
		name string
		id   uint32
	}{
		{mtproto.Predicate_invokeWithLayer, 0xda9b0d0d},
		{mtproto.Predicate_user, 0xb1b8cc83},
		{mtproto.Predicate_userFull, 0xb90c2afd},
		{mtproto.Predicate_message, 0x7600b9d3},
		{mtproto.Predicate_inputReplyToMessage, 0x75c1ad70},
		{mtproto.Predicate_messageReplyHeader, 0xddb5ab75},
		{mtproto.Predicate_channel, 0x7ad15240},
		{mtproto.Predicate_auth_sendCode, 0xa677244f},
		{mtproto.Predicate_messages_sendMessage, 0xfef48f62},
	}
	for _, c := range checks {
		got := uint32(mtproto.GetClazzID(c.name, mtproto.Layer))
		if got == 0 {
			t.Errorf("GetClazzID(%s, 232) = 0", c.name)
		}
		if got != c.id {
			t.Errorf("GetClazzID(%s, 232) = 0x%x, want 0x%x", c.name, got, c.id)
		}
	}
}
