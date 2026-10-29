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
		{mtproto.Predicate_phoneCallWaiting, 0xc5226f17},
		{mtproto.Predicate_phoneCallRequested, 0x14b0ed0c},
		{mtproto.Predicate_phoneCallAccepted, 0x3660c311},
		{mtproto.Predicate_phoneCall, 0x30535af5},
		{mtproto.Predicate_phoneCallDiscarded, 0x50ca4de1},
		{mtproto.Predicate_phoneConnection, 0x9cc123c7},
		{mtproto.Predicate_phoneConnectionWebrtc, 0x635fe375},
		{mtproto.Predicate_updatePhoneCall, 0xab0f6b1e},
		{mtproto.Predicate_updatePhoneCallSignalingData, 0x2661bf09},
		{mtproto.Predicate_phone_getCallConfig, 0x55451fa9},
		{mtproto.Predicate_phone_requestCall, 0x42ff96ed},
		{mtproto.Predicate_groupCall, 0xefb2b617},
		{mtproto.Predicate_updateGroupCall, 0x9d2216e0},
		{mtproto.Predicate_updateGroupCallParticipants, 0xf2ebdb4e},
		{mtproto.Predicate_updateGroupCallConnection, 0x0b783982},
		{mtproto.Predicate_messages_dhConfig, 0x2c221edd},
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
