package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAuthFinishPasskeyLoginDoesNotError(t *testing.T) {
	got, err := (&PasskeyCore{}).AuthFinishPasskeyLogin(&mtproto.TLAuthFinishPasskeyLogin{})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_auth_authorizationSignUpRequired {
		t.Fatal(got.GetPredicateName())
	}
	if got.GetUser() != nil {
		t.Fatal("passkey stub must not return a user")
	}
	buf := mtproto.NewEncodeBuf(64)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
}
