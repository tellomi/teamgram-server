package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// Phones 9996610461 and 9996610469 are this task's range.
// 9996610469 is only used as an unregistered change-phone target.
// account.changePhone is never called.

func TestCancelCodeBlocksSignIn(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)

	const phone = "9996610461"
	sent, err := c.Invoke(ctx, &mtproto.TLAuthSendCode{
		Constructor: mtproto.CRC32_auth_sendCode,
		PhoneNumber: phone,
		ApiId:       cfg.APIID,
		ApiHash:     cfg.APIHash,
		Settings:    mtproto.MakeTLCodeSettings(&mtproto.CodeSettings{}).To_CodeSettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	code, ok := sent.(*mtproto.TLAuthSentCode)
	if !ok || code.GetPhoneCodeHash() == "" {
		t.Fatalf("auth.sendCode returned %T", sent)
	}
	if _, err = c.Invoke(ctx, &mtproto.TLAuthCancelCode{
		Constructor:   mtproto.CRC32_auth_cancelCode,
		PhoneNumber:   phone,
		PhoneCodeHash: code.GetPhoneCodeHash(),
	}); err != nil {
		t.Fatal(err)
	}
	_, err = c.Invoke(ctx, &mtproto.TLAuthSignIn{
		Constructor:          mtproto.CRC32_auth_signIn_8d52a951,
		PhoneNumber:          phone,
		PhoneCodeHash:        code.GetPhoneCodeHash(),
		PhoneCode_FLAGSTRING: strFlag("11111"),
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Code != 400 {
		t.Fatalf("signIn after cancelCode: %v", err)
	}
}

func TestExportImportAuthorization(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	owner, self := dialPhone(t, ctx, cfg, "9996610461", "ExportAuth")

	exported, err := owner.Invoke(ctx, &mtproto.TLAuthExportAuthorization{
		Constructor: mtproto.CRC32_auth_exportAuthorization,
		DcId:        1,
	})
	if err != nil {
		t.Fatal(err)
	}
	exp, ok := exported.(*mtproto.TLAuthExportedAuthorization)
	if !ok || exp.GetId() != self.ID || len(exp.GetBytes()) < 16 || string(exp.GetBytes()) == "1" {
		t.Fatalf("export: %T id=%d bytes=%q", exported, exp.GetId(), exp.GetBytes())
	}

	fresh, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Close)
	imported, err := fresh.Invoke(ctx, &mtproto.TLAuthImportAuthorization{
		Constructor: mtproto.CRC32_auth_importAuthorization,
		Id:          exp.GetId(),
		Bytes:       exp.GetBytes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	authz, ok := imported.(*mtproto.TLAuthAuthorization)
	if !ok || authz.GetUser().GetId() != self.ID {
		t.Fatalf("import: %T user=%d", imported, authz.GetUser().GetId())
	}
	users, err := fresh.Invoke(ctx, &mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id: []*mtproto.InputUser{
			mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if userByID(usersOf(users), self.ID) == nil {
		t.Fatalf("session after import: %T", users)
	}

	_, err = fresh.Invoke(ctx, &mtproto.TLAuthImportAuthorization{
		Constructor: mtproto.CRC32_auth_importAuthorization,
		Id:          exp.GetId(),
		Bytes:       exp.GetBytes(),
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "AUTH_BYTES_INVALID" {
		t.Fatalf("reuse: %v", err)
	}
	_, err = fresh.Invoke(ctx, &mtproto.TLAuthImportAuthorization{
		Constructor: mtproto.CRC32_auth_importAuthorization,
		Id:          self.ID,
		Bytes:       []byte("not-a-real-export"),
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "AUTH_BYTES_INVALID" {
		t.Fatalf("garbage: %v", err)
	}
}

func TestImportLoginTokenInvalid(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610461", "LoginToken")
	_, err := c.Invoke(ctx, &mtproto.TLAuthImportLoginToken{
		Constructor: mtproto.CRC32_auth_importLoginToken,
		Token:       []byte{1, 2, 3, 4},
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "AUTH_TOKEN_INVALID" {
		t.Fatalf("importLoginToken: %v", err)
	}
}

func TestSendChangePhoneCodeAcceptsTestNumber(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// 9996610469 stays unregistered. Do not call account.changePhone.
	c, _ := dialPhone(t, ctx, cfg, "9996610461", "ChangePhone")
	sent, err := c.Invoke(ctx, &mtproto.TLAccountSendChangePhoneCode{
		Constructor: mtproto.CRC32_account_sendChangePhoneCode,
		PhoneNumber: "9996610469",
		Settings:    mtproto.MakeTLCodeSettings(&mtproto.CodeSettings{}).To_CodeSettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	code, ok := sent.(*mtproto.TLAuthSentCode)
	if !ok || code.GetPhoneCodeHash() == "" {
		t.Fatalf("account.sendChangePhoneCode returned %T", sent)
	}
}
