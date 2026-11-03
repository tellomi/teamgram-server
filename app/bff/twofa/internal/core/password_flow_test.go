package core

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	"github.com/teamgram/teamgram-server/pkg/srp"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type sentMail struct {
	to   string
	code string
}

func TestCloudPasswordSetCheckChangeRemove(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var mails []sentMail
	svc := password.New(password.Options{
		Now: func() time.Time { return now },
		SendEmail: func(_ context.Context, to, code string) (string, error) {
			mails = append(mails, sentMail{to: to, code: code})
			return code, nil
		},
	})
	prev := password.Swap(svc)
	t.Cleanup(func() { password.Swap(prev) })

	c := testCore()
	c.MD = &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 7}
	empty := mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP()

	if _, err := c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{Password: empty}); err != mtproto.ErrNewSettingsEmpty {
		t.Fatalf("empty: %v", err)
	}

	n, err := setPassword(t, c, "secret", "hint", "recover@tellomi.app")
	if statusText(err) != "EMAIL_UNCONFIRMED_6" || n != 0 {
		t.Fatalf("enroll %d %v mails=%v", n, err, mails)
	}
	if len(mails) != 1 || mails[0].to != "recover@tellomi.app" || len(mails[0].code) != 6 {
		t.Fatalf("mail %#v", mails)
	}
	pw, err := c.AccountGetPassword(nil)
	if err != nil || !pw.GetHasPassword() || pw.GetHasRecovery() {
		t.Fatalf("unconfirmed password: has=%v recovery=%v err=%v", pw.GetHasPassword(), pw.GetHasRecovery(), err)
	}
	if pw.GetEmailUnconfirmedPattern().GetValue() != "r***@tellomi.app" || pw.GetHint().GetValue() != "hint" {
		t.Fatalf("pattern/hint %+v", pw)
	}
	if len(pw.GetSrp_B()) != 256 || pw.GetSrpId().GetValue() == 0 || pw.GetCurrentAlgo() == nil {
		t.Fatal("missing srp material")
	}
	if _, err = c.AccountConfirmPasswordEmail(&mtproto.TLAccountConfirmPasswordEmail{Code: "000000"}); err != mtproto.ErrCodeInvalid {
		t.Fatalf("bad code: %v", err)
	}
	if _, err = c.AccountConfirmPasswordEmail(&mtproto.TLAccountConfirmPasswordEmail{Code: mails[0].code}); err != nil {
		t.Fatal(err)
	}
	pw, err = c.AccountGetPassword(nil)
	if err != nil || !pw.GetHasRecovery() || pw.GetEmailUnconfirmedPattern() != nil {
		t.Fatalf("confirmed: %+v %v", pw, err)
	}

	if err = password.Verify(context.Background(), 42, 7, proofFor(t, pw, "nope")); err != mtproto.ErrPasswordHashInvalid {
		t.Fatalf("wrong password: %v", err)
	}
	pw, err = c.AccountGetPassword(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = password.Verify(context.Background(), 42, 7, proofFor(t, pw, "secret")); err != nil {
		t.Fatal(err)
	}
	pendingErr := password.NoteLogin(context.Background(), 7, 42)
	if pendingErr != nil {
		t.Fatal(pendingErr)
	}
	gotUser, err := password.PendingUser(context.Background(), 7)
	if err != nil || gotUser != 42 {
		t.Fatalf("pending %d %v", gotUser, err)
	}
	if err = password.ClearLogin(context.Background(), 7); err != nil {
		t.Fatal(err)
	}

	settings, err := c.AccountGetPasswordSettings(&mtproto.TLAccountGetPasswordSettings{Password: proofFor(t, mustPassword(t, c), "secret")})
	if err != nil || settings.GetEmail().GetValue() != "recover@tellomi.app" {
		t.Fatalf("settings %+v %v", settings, err)
	}

	changePassword(t, c, "secret", "next")
	if err = password.Verify(context.Background(), 42, 7, proofFor(t, mustPassword(t, c), "secret")); err != mtproto.ErrPasswordHashInvalid {
		t.Fatalf("old password still worked: %v", err)
	}
	if err = password.Verify(context.Background(), 42, 7, proofFor(t, mustPassword(t, c), "next")); err != nil {
		t.Fatal(err)
	}

	stale := mustPassword(t, c)
	changePassword(t, c, "next", "after")
	if err = password.Verify(context.Background(), 42, 7, proofFor(t, stale, "next")); err != mtproto.ErrSrpPasswordChanged {
		t.Fatalf("stale challenge: %v", err)
	}

	for i := 0; i < 5; i++ {
		err = password.Verify(context.Background(), 42, 7, proofFor(t, mustPassword(t, c), "wrong"))
		if err != mtproto.ErrPasswordHashInvalid {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if statusText(password.Verify(context.Background(), 42, 7, proofFor(t, mustPassword(t, c), "wrong"))) == "" {
		t.Fatal("expected FLOOD_WAIT")
	}
	if got := statusText(password.Verify(context.Background(), 42, 7, proofFor(t, mustPassword(t, c), "after"))); len(got) < 11 || got[:10] != "FLOOD_WAIT" {
		t.Fatalf("flood: %s", got)
	}

	now = now.Add(2 * time.Minute)
	ok, err := c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{
		Password:    proofFor(t, mustPassword(t, c), "after"),
		NewSettings: &mtproto.Account_PasswordInputSettings{},
	})
	if err != nil || ok != mtproto.BoolTrue {
		t.Fatalf("remove: %v %v", ok, err)
	}
	pw, err = c.AccountGetPassword(nil)
	if err != nil || pw.GetHasPassword() {
		t.Fatalf("removed: %+v %v", pw, err)
	}
	if err = password.Verify(context.Background(), 42, 7, empty); err != mtproto.ErrPasswordMissing {
		t.Fatalf("missing: %v", err)
	}
}

func TestPasswordResetAndRecoveryEmail(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	svc := password.New(password.Options{Now: func() time.Time { return now }})
	prev := password.Swap(svc)
	t.Cleanup(func() { password.Swap(prev) })

	c := testCore()
	c.MD = &metadata.RpcMetadata{UserId: 9, PermAuthKeyId: 3}
	setPassword(t, c, "secret", "", "")

	res, err := password.Reset(context.Background(), 9)
	if err != nil || res.GetPredicateName() != mtproto.Predicate_account_resetPasswordRequestedWait {
		t.Fatalf("schedule %+v %v", res, err)
	}
	if res.GetUntilDate() != int32(now.Add(7*24*time.Hour).Unix()) {
		t.Fatalf("until %d", res.GetUntilDate())
	}
	again, err := password.Reset(context.Background(), 9)
	if err != nil || again.GetUntilDate() != res.GetUntilDate() {
		t.Fatalf("repeat %+v %v", again, err)
	}
	pw, err := c.AccountGetPassword(nil)
	if err != nil || pw.GetPendingResetDate().GetValue() != res.GetUntilDate() {
		t.Fatalf("pending on getPassword: %+v %v", pw.GetPendingResetDate(), err)
	}
	if _, err = c.AccountDeclinePasswordReset(nil); err != nil {
		t.Fatal(err)
	}
	if _, err = c.AccountDeclinePasswordReset(nil); err != mtproto.ErrResetRequestMissing {
		t.Fatalf("decline: %v", err)
	}

	if _, err = password.Reset(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
	now = now.Add(8 * 24 * time.Hour)
	done, err := password.Reset(context.Background(), 9)
	if err != nil || done.GetPredicateName() != mtproto.Predicate_account_resetPasswordOk {
		t.Fatalf("done %+v %v", done, err)
	}
	has, err := password.HasPassword(context.Background(), 9)
	if err != nil || has {
		t.Fatalf("has=%v %v", has, err)
	}

	setPassword(t, c, "secret", "", "")
	pw, err = c.AccountGetPassword(nil)
	if err != nil {
		t.Fatal(err)
	}
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{7}, 32)...)
	salt2 := pw.GetNewAlgo().GetSalt2()
	next := srp.Verifier([]byte("recovered"), salt1, salt2)
	settings := &mtproto.Account_PasswordInputSettings{
		NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
			Salt1: salt1,
			Salt2: append([]byte{}, salt2...),
			G:     pw.GetNewAlgo().GetG(),
			P:     append([]byte{}, pw.GetNewAlgo().GetP()...),
		}).To_PasswordKdfAlgo(),
		NewPasswordHash: next,
		Email:           &wrapperspb.StringValue{Value: "new@tellomi.app"},
	}
	if err = password.CheckRecoverySettings(context.Background(), 9, settings); err != nil {
		t.Fatal(err)
	}
	if err = password.CommitRecovery(context.Background(), 9, settings); err != nil {
		t.Fatal(err)
	}
	email, has, err := password.RecoveryEmail(context.Background(), 9)
	if err != nil || !has || email != "new@tellomi.app" {
		t.Fatalf("recovery email %q %v %v", email, has, err)
	}
	if err = password.Verify(context.Background(), 9, 3, proofFor(t, mustPassword(t, c), "recovered")); err != nil {
		t.Fatal(err)
	}
}

func setPassword(t *testing.T, c *TwoFaCore, pass, hint, email string) (int32, error) {
	t.Helper()
	pw, err := c.AccountGetPassword(nil)
	if err != nil {
		t.Fatal(err)
	}
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{0x11}, 32)...)
	settings := &mtproto.Account_PasswordInputSettings{
		NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
			Salt1: salt1,
			Salt2: append([]byte{}, pw.GetNewAlgo().GetSalt2()...),
			G:     pw.GetNewAlgo().GetG(),
			P:     append([]byte{}, pw.GetNewAlgo().GetP()...),
		}).To_PasswordKdfAlgo(),
		NewPasswordHash: srp.Verifier([]byte(pass), salt1, pw.GetNewAlgo().GetSalt2()),
	}
	if hint != "" {
		settings.Hint = &wrapperspb.StringValue{Value: hint}
	}
	if email != "" {
		settings.Email = &wrapperspb.StringValue{Value: email}
	}
	_, err = c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{
		Password:    mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP(),
		NewSettings: settings,
	})
	return 0, err
}

func changePassword(t *testing.T, c *TwoFaCore, old, next string) {
	t.Helper()
	pw := mustPassword(t, c)
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{0x22}, 32)...)
	_, err := c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{
		Password: proofFor(t, pw, old),
		NewSettings: &mtproto.Account_PasswordInputSettings{
			NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
				Salt1: salt1,
				Salt2: append([]byte{}, pw.GetNewAlgo().GetSalt2()...),
				G:     pw.GetNewAlgo().GetG(),
				P:     append([]byte{}, pw.GetNewAlgo().GetP()...),
			}).To_PasswordKdfAlgo(),
			NewPasswordHash: srp.Verifier([]byte(next), salt1, pw.GetNewAlgo().GetSalt2()),
			Hint:            &wrapperspb.StringValue{Value: "h"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustPassword(t *testing.T, c *TwoFaCore) *mtproto.Account_Password {
	t.Helper()
	pw, err := c.AccountGetPassword(nil)
	if err != nil {
		t.Fatal(err)
	}
	return pw
}

func proofFor(t *testing.T, pw *mtproto.Account_Password, pass string) *mtproto.InputCheckPasswordSRP {
	t.Helper()
	a := bytes.Repeat([]byte{0x3c}, 256)
	A, M1, err := srp.ClientProof([]byte(pass), pw.GetCurrentAlgo().GetSalt1(), pw.GetCurrentAlgo().GetSalt2(), a, pw.GetSrp_B())
	if err != nil {
		t.Fatal(err)
	}
	return mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{
		SrpId: pw.GetSrpId().GetValue(),
		A:     A,
		M1:    M1,
	}).To_InputCheckPasswordSRP()
}

func TestSecureSettingsRoundTrip(t *testing.T) {
	svc := password.New(password.Options{})
	prev := password.Swap(svc)
	t.Cleanup(func() { password.Swap(prev) })
	c := testCore()
	c.MD = &metadata.RpcMetadata{UserId: 11, PermAuthKeyId: 4}
	pw := mustPassword(t, c)
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{0x44}, 32)...)
	secret := bytes.Repeat([]byte{0x5a}, 32)
	_, err := c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{
		Password: mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP(),
		NewSettings: &mtproto.Account_PasswordInputSettings{
			NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
				Salt1: salt1,
				Salt2: append([]byte{}, pw.GetNewAlgo().GetSalt2()...),
				G:     pw.GetNewAlgo().GetG(),
				P:     append([]byte{}, pw.GetNewAlgo().GetP()...),
			}).To_PasswordKdfAlgo(),
			NewPasswordHash: srp.Verifier([]byte("secret"), salt1, pw.GetNewAlgo().GetSalt2()),
			NewSecureSettings: mtproto.MakeTLSecureSecretSettings(&mtproto.SecureSecretSettings{
				SecureAlgo:     pw.GetNewSecureAlgo(),
				SecureSecret:   secret,
				SecureSecretId: 99,
			}).To_SecureSecretSettings(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.AccountGetPassword(nil)
	if err != nil || !got.GetHasSecureValues() {
		t.Fatalf("has_secure_values %+v %v", got, err)
	}
	settings, err := c.AccountGetPasswordSettings(&mtproto.TLAccountGetPasswordSettings{
		Password: proofFor(t, mustPassword(t, c), "secret"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if settings.GetSecureSettings().GetSecureSecretId() != 99 || !bytes.Equal(settings.GetSecureSettings().GetSecureSecret(), secret) {
		t.Fatalf("%+v", settings.GetSecureSettings())
	}
}

func TestGetPasswordUsesPendingUserAfterLogout(t *testing.T) {
	svc := password.New(password.Options{})
	prev := password.Swap(svc)
	t.Cleanup(func() { password.Swap(prev) })

	c := testCore()
	c.MD = &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 7}
	if _, err := setPassword(t, c, "secret", "hint", ""); err != nil {
		t.Fatal(err)
	}
	if err := password.NoteLogin(context.Background(), 7, 42); err != nil {
		t.Fatal(err)
	}

	// auth.logOut cleared the session user id. The auth key still owes a password.
	c.MD = &metadata.RpcMetadata{UserId: 0, PermAuthKeyId: 7}
	pw, err := c.AccountGetPassword(nil)
	if err != nil || !pw.GetHasPassword() || pw.GetCurrentAlgo() == nil || len(pw.GetSrp_B()) != 256 || pw.GetSrpId().GetValue() == 0 {
		t.Fatalf("pending getPassword: has=%v algo=%v b=%d id=%d err=%v", pw.GetHasPassword(), pw.GetCurrentAlgo(), len(pw.GetSrp_B()), pw.GetSrpId().GetValue(), err)
	}
	if err = password.Verify(context.Background(), 42, 7, proofFor(t, pw, "secret")); err != nil {
		t.Fatal(err)
	}

	c.MD.PermAuthKeyId = 8
	plain, err := c.AccountGetPassword(nil)
	if err != nil || plain.GetHasPassword() || plain.GetCurrentAlgo() != nil {
		t.Fatalf("unrelated key: %+v %v", plain, err)
	}
}

func TestCheckPasswordAcceptsProofIssuedToAnotherAuthKey(t *testing.T) {
	svc := password.New(password.Options{})
	prev := password.Swap(svc)
	t.Cleanup(func() { password.Swap(prev) })

	c := testCore()
	c.MD = &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 7}
	if _, err := setPassword(t, c, "secret", "hint", ""); err != nil {
		t.Fatal(err)
	}
	pw, err := c.AccountGetPassword(nil)
	if err != nil {
		t.Fatal(err)
	}
	// iOS does not retry SRP_ID_INVALID when the temp key and the permanent
	// key disagree between getPassword and checkPassword.
	if err = password.Verify(context.Background(), 42, 99, proofFor(t, pw, "secret")); err != nil {
		t.Fatal(err)
	}
}

func TestGetPasswordWithoutAuthKeyDoesNotLookLikeNoPassword(t *testing.T) {
	c := testCore()
	c.MD = &metadata.RpcMetadata{UserId: 0, PermAuthKeyId: 0}
	_, err := c.AccountGetPassword(nil)
	if status.Convert(err).Message() != "AUTH_KEY_PERM_EMPTY" {
		t.Fatalf("got %v", err)
	}
}

func TestDisablePasswordAndroidAndIOSShapes(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var codes []string
	svc := password.New(password.Options{
		Now: func() time.Time { return now },
		SendEmail: func(_ context.Context, _, code string) (string, error) {
			codes = append(codes, code)
			return code, nil
		},
	})
	prev := password.Swap(svc)
	t.Cleanup(func() { password.Swap(prev) })

	shapes := []struct {
		name    string
		userID  int64
		authKey int64
		build   func() *mtproto.Account_PasswordInputSettings
	}{
		{
			name:    "android",
			userID:  42,
			authKey: 7,
			build: func() *mtproto.Account_PasswordInputSettings {
				return &mtproto.Account_PasswordInputSettings{
					NewAlgo:         mtproto.MakeTLPasswordKdfAlgoUnknown(nil).To_PasswordKdfAlgo(),
					NewPasswordHash: []byte{},
					Hint:            &wrapperspb.StringValue{Value: ""},
				}
			},
		},
		{
			name:    "ios",
			userID:  43,
			authKey: 8,
			build: func() *mtproto.Account_PasswordInputSettings {
				return &mtproto.Account_PasswordInputSettings{
					NewAlgo:         mtproto.MakeTLPasswordKdfAlgoUnknown(nil).To_PasswordKdfAlgo(),
					NewPasswordHash: nil,
					Hint:            &wrapperspb.StringValue{Value: ""},
					Email:           &wrapperspb.StringValue{Value: ""},
				}
			},
		},
	}
	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			c := testCore()
			c.MD = &metadata.RpcMetadata{UserId: shape.userID, PermAuthKeyId: shape.authKey}
			enrollConfirmedPassword(t, c, &codes)
			if err := password.NoteLogin(context.Background(), shape.authKey, shape.userID); err != nil {
				t.Fatal(err)
			}
			bad, err := c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{
				Password:    proofFor(t, mustPassword(t, c), "nope"),
				NewSettings: shape.build(),
			})
			if err != mtproto.ErrPasswordHashInvalid || bad != nil {
				t.Fatalf("wrong password: %v %v", bad, err)
			}
			if has, _ := password.HasPassword(context.Background(), shape.userID); !has {
				t.Fatal("wrong password removed 2FA")
			}
			unknown := &mtproto.PasswordKdfAlgo{Constructor: mtproto.CRC32_passwordKdfAlgoUnknown}
			if _, err = c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{
				Password: proofFor(t, mustPassword(t, c), "secret"),
				NewSettings: &mtproto.Account_PasswordInputSettings{
					NewAlgo:         unknown,
					NewPasswordHash: []byte{1},
				},
			}); err != mtproto.ErrNewSaltInvalid {
				t.Fatalf("unknown algo with a hash: %v", err)
			}

			ok, err := c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{
				Password:    proofFor(t, mustPassword(t, c), "secret"),
				NewSettings: shape.build(),
			})
			if err != nil || ok != mtproto.BoolTrue {
				t.Fatalf("disable: %v %v", ok, err)
			}
			pw, err := c.AccountGetPassword(nil)
			if err != nil || pw.GetHasPassword() || pw.GetHasRecovery() || pw.GetHasSecureValues() || pw.GetCurrentAlgo() != nil {
				t.Fatalf("after disable: %+v %v", pw, err)
			}
			email, has, err := password.RecoveryEmail(context.Background(), shape.userID)
			if err != nil || has || email != "" {
				t.Fatalf("recovery email %q has=%v %v", email, has, err)
			}
			pending, err := password.PendingUser(context.Background(), shape.authKey)
			if err != nil || pending != 0 {
				t.Fatalf("pending login %d %v", pending, err)
			}
			// The next auth.signIn has no verifier, so it does not ask for a password.
			if has, err = password.HasPassword(context.Background(), shape.userID); err != nil || has {
				t.Fatalf("login would still need a password: %v %v", has, err)
			}
		})
	}
}

func enrollConfirmedPassword(t *testing.T, c *TwoFaCore, codes *[]string) {
	t.Helper()
	pw := mustPassword(t, c)
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{0x11}, 32)...)
	secret := bytes.Repeat([]byte{0x5a}, 32)
	_, err := c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{
		Password: mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP(),
		NewSettings: &mtproto.Account_PasswordInputSettings{
			NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
				Salt1: salt1,
				Salt2: append([]byte{}, pw.GetNewAlgo().GetSalt2()...),
				G:     pw.GetNewAlgo().GetG(),
				P:     append([]byte{}, pw.GetNewAlgo().GetP()...),
			}).To_PasswordKdfAlgo(),
			NewPasswordHash: srp.Verifier([]byte("secret"), salt1, pw.GetNewAlgo().GetSalt2()),
			Hint:            &wrapperspb.StringValue{Value: "hint"},
			Email:           &wrapperspb.StringValue{Value: "recover@tellomi.app"},
			NewSecureSettings: mtproto.MakeTLSecureSecretSettings(&mtproto.SecureSecretSettings{
				SecureAlgo:     pw.GetNewSecureAlgo(),
				SecureSecret:   secret,
				SecureSecretId: 99,
			}).To_SecureSecretSettings(),
		},
	})
	if statusText(err) != "EMAIL_UNCONFIRMED_6" || len(*codes) == 0 {
		t.Fatalf("enroll: %v codes=%v", err, *codes)
	}
	if _, err = c.AccountConfirmPasswordEmail(&mtproto.TLAccountConfirmPasswordEmail{Code: (*codes)[len(*codes)-1]}); err != nil {
		t.Fatal(err)
	}
}

func statusText(err error) string {
	if err == nil {
		return ""
	}
	if st, ok := status.FromError(err); ok {
		return st.Message()
	}
	return err.Error()
}
