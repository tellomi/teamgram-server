package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestWiredEmailIdentityRejectsASecondAddress(t *testing.T) {
	prev := password.Swap(password.New(password.Options{}))
	t.Cleanup(func() { password.Swap(prev) })
	password.SeedIdentity(7, "user@tellomi.app")

	c := &AuthorizationCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 7, PermAuthKeyId: 3},
	}
	_, err := c.AccountSendVerifyEmailCode(&mtproto.TLAccountSendVerifyEmailCode{
		Email: "other@tellomi.app",
		Purpose: mtproto.MakeTLEmailVerifyPurposeLoginChange(&mtproto.EmailVerifyPurpose{}).
			To_EmailVerifyPurpose(),
	})
	st, _ := status.FromError(err)
	if st.Message() != "EMAIL_NOT_ALLOWED" {
		t.Fatal(err)
	}
	got, gerr := c.codes().LoginEmail(context.Background(), 7)
	if gerr != nil || got != "" {
		t.Fatalf("redis %q %v", got, gerr)
	}

	_, err = c.AccountSendVerifyEmailCode(&mtproto.TLAccountSendVerifyEmailCode{
		Email: "user@tellomi.app",
		Purpose: mtproto.MakeTLEmailVerifyPurposeLoginChange(&mtproto.EmailVerifyPurpose{}).
			To_EmailVerifyPurpose(),
	})
	st, _ = status.FromError(err)
	if st.Message() == "EMAIL_NOT_ALLOWED" {
		t.Fatal(err)
	}
}
