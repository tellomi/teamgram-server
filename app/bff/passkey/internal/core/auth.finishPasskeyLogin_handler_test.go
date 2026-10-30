package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func testPasskeyCore() *PasskeyCore {
	return &PasskeyCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
	}
}

func rpcCode(err error) (code uint32, msg string) {
	st, ok := status.FromError(err)
	if !ok {
		return 0, ""
	}
	return uint32(st.Code()), st.Message()
}

func credential() *mtproto.InputPasskeyCredential {
	return mtproto.MakeTLInputPasskeyCredentialPublicKey(&mtproto.InputPasskeyCredential{
		Id:    "cred",
		RawId: "raw",
		Response: mtproto.MakeTLInputPasskeyResponseLogin(&mtproto.InputPasskeyResponse{
			ClientData:        mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: "{}"}).To_DataJSON(),
			AuthenticatorData: []byte{1},
			Signature:         []byte{2},
			UserHandle:        "2:1",
		}).To_InputPasskeyResponse(),
	}).To_InputPasskeyCredential()
}

func TestFinishPasskeyLoginNotFound(t *testing.T) {
	auth, err := testPasskeyCore().AuthFinishPasskeyLogin(&mtproto.TLAuthFinishPasskeyLogin{
		Credential: credential(),
	})
	if auth != nil {
		t.Fatal("passkey login must not return auth.authorization")
	}
	code, msg := rpcCode(err)
	if code != 400 || msg != "PASSKEY_CREDENTIAL_NOT_FOUND" {
		t.Fatalf("code %d msg %s", code, msg)
	}
}

func TestFinishPasskeyLoginCredentialInvalid(t *testing.T) {
	_, err := testPasskeyCore().AuthFinishPasskeyLogin(nil)
	code, msg := rpcCode(err)
	if code != 400 || msg != "CREDENTIAL_INVALID" {
		t.Fatalf("nil: code %d msg %s", code, msg)
	}
	_, err = testPasskeyCore().AuthFinishPasskeyLogin(&mtproto.TLAuthFinishPasskeyLogin{})
	code, msg = rpcCode(err)
	if code != 400 || msg != "CREDENTIAL_INVALID" {
		t.Fatalf("empty: code %d msg %s", code, msg)
	}
}
