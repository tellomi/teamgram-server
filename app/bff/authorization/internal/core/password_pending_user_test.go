package core

import (
	"bytes"
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	"github.com/teamgram/teamgram-server/pkg/srp"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestRecoveryUsesPendingUserWhenSessionUserIsZero(t *testing.T) {
	svc := password.New(password.Options{
		SendEmail: func(context.Context, string, string) (string, error) {
			return "654321", nil
		},
	})
	prev := password.Swap(svc)
	t.Cleanup(func() { password.Swap(prev) })
	password.SeedPhone(42)

	ctx := context.Background()
	pw, err := password.AccountPassword(ctx, 42, 7)
	if err != nil {
		t.Fatal(err)
	}
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{0x11}, 32)...)
	n, err := password.Update(ctx, 42, 7, mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP(), &mtproto.Account_PasswordInputSettings{
		NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
			Salt1: salt1,
			Salt2: append([]byte{}, pw.GetNewAlgo().GetSalt2()...),
			G:     pw.GetNewAlgo().GetG(),
			P:     append([]byte{}, pw.GetNewAlgo().GetP()...),
		}).To_PasswordKdfAlgo(),
		NewPasswordHash: srp.Verifier([]byte("secret"), salt1, pw.GetNewAlgo().GetSalt2()),
		Email:           &wrapperspb.StringValue{Value: "recover@tellomi.app"},
	})
	if err != nil || n != 6 {
		t.Fatalf("enroll %d %v", n, err)
	}
	if err = password.ConfirmEmail(ctx, 42, "654321"); err != nil {
		t.Fatal(err)
	}
	if err = password.NoteLogin(ctx, 7, 42); err != nil {
		t.Fatal(err)
	}

	c := &AuthorizationCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 0, PermAuthKeyId: 7},
	}
	id, err := c.passwordUserID()
	if err != nil || id != 42 {
		t.Fatalf("pending user %d %v", id, err)
	}
	// Delivery is not configured on this core. Reaching the sender means the
	// user id was resolved; user id 0 would have been PASSWORD_RECOVERY_NA.
	_, err = c.AuthRequestPasswordRecovery(nil)
	if status.Convert(err).Message() != "EMAIL_NOT_SETUP" {
		t.Fatalf("request recovery: %v", err)
	}

	c.MD.PermAuthKeyId = 99
	_, err = c.AuthRequestPasswordRecovery(nil)
	if status.Convert(err).Message() != "PASSWORD_RECOVERY_NA" {
		t.Fatalf("no pending row: %v", err)
	}
	_, err = c.AuthCheckRecoveryPassword(&mtproto.TLAuthCheckRecoveryPassword{Code: "000000"})
	if status.Convert(err).Message() != "PASSWORD_RECOVERY_EXPIRED" {
		t.Fatalf("check recovery without a user: %v", err)
	}
	_, err = c.AuthRecoverPassword(&mtproto.TLAuthRecoverPassword{Code: "000000"})
	if status.Convert(err).Message() != "PASSWORD_RECOVERY_EXPIRED" {
		t.Fatalf("recover without a user: %v", err)
	}
}
