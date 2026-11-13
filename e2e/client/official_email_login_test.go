package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestOfficialPhoneEmailLogin is the server half of Telegram-iOS PR 186.
// A 99966 number with no login email still gets an SMS code. After Settings
// binds an address, the next auth.sendCode is sentCodeTypeEmailCode and
// auth.signIn uses email_verification. Cloud password still sits after that
// code. The legacy path (an email address in phone_number) still returns
// sentCodeTypeEmailCode.
func TestOfficialPhoneEmailLogin(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	legacy := "e2e-legacy-probe@tellomi.test"
	legacyClient, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(legacyClient.Close)
	legacySent := sendAuthCode(t, ctx, legacyClient, legacy)
	if legacySent.GetType().GetPredicateName() != mtproto.Predicate_auth_sentCodeTypeEmailCode {
		t.Fatalf("legacy email sendCode type %s", legacySent.GetType().GetPredicateName())
	}
	if legacySent.GetType().GetEmailPattern() != "e***@tellomi.test" || legacySent.GetType().GetLength() < 4 {
		t.Fatalf("legacy sent code %+v", legacySent.GetType())
	}

	control, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(control.Close)
	const controlPhone = "9996610182"
	controlSent := sendAuthCode(t, ctx, control, controlPhone)
	if controlSent.GetType().GetPredicateName() != mtproto.Predicate_auth_sentCodeTypeSms {
		t.Fatalf("default 99966 sendCode type %s", controlSent.GetType().GetPredicateName())
	}
	if _, err = finishPhoneLogin(ctx, control, controlPhone, controlSent.GetPhoneCodeHash(), "11111", "EmailCtrl"); err != nil {
		t.Fatal(err)
	}

	const (
		phone    = "9996610181"
		email    = "e2e-9996610181@tellomi.test"
		pattern  = "e***@tellomi.test"
		password = "e2e-email-secret"
	)
	client, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if _, err = loginPhoneAccount(t, ctx, client, phone, email, password); err != nil {
		t.Fatal(err)
	}
	if err = ensureLoginEmail(t, ctx, client, email, pattern); err != nil {
		t.Fatal(err)
	}
	pw, err := getPassword(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	if pw.GetLoginEmailPattern().GetValue() != pattern {
		t.Fatalf("login_email_pattern %q", pw.GetLoginEmailPattern().GetValue())
	}
	if !pw.GetHasPassword() {
		if err = setCloudPassword(ctx, client, password); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = client.Invoke(ctx, &mtproto.TLAuthLogOut{Constructor: mtproto.CRC32_auth_logOut}); err != nil {
		t.Fatal(err)
	}
	client.Close()

	fresh, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Close)
	before := emailCodeCount(t, email)
	sent := sendAuthCode(t, ctx, fresh, phone)
	if sent.GetType().GetPredicateName() != mtproto.Predicate_auth_sentCodeTypeEmailCode {
		t.Fatalf("phone with login email type %s", sent.GetType().GetPredicateName())
	}
	if sent.GetType().GetEmailPattern() != pattern {
		t.Fatalf("pattern %q", sent.GetType().GetEmailPattern())
	}
	if sent.GetType().GetAppleSigninAllowed() || sent.GetType().GetGoogleSigninAllowed() {
		t.Fatal("apple and google sign-in stay off")
	}
	hash := sent.GetPhoneCodeHash()
	_, err = signInEmail(ctx, fresh, phone, hash, mtproto.MakeTLEmailVerificationApple(&mtproto.EmailVerification{
		Token: "not-a-token",
	}).To_EmailVerification())
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "EMAIL_INVALID" {
		t.Fatalf("apple token: %v", err)
	}
	_, err = signInEmail(ctx, fresh, phone, hash, mtproto.MakeTLEmailVerificationCode(&mtproto.EmailVerification{
		Code: "000000",
	}).To_EmailVerification())
	rpc, ok = asRPCError(err)
	if !ok || rpc.Message != "PHONE_CODE_INVALID" {
		t.Fatalf("wrong email code: %v", err)
	}
	code := waitEmailCode(t, email, before)
	_, err = signInEmail(ctx, fresh, phone, hash, mtproto.MakeTLEmailVerificationCode(&mtproto.EmailVerification{
		Code: code,
	}).To_EmailVerification())
	rpc, ok = asRPCError(err)
	if !ok || rpc.Message != "SESSION_PASSWORD_NEEDED" {
		t.Fatalf("email signIn: %v", err)
	}
	if _, err = acceptCloudPassword(ctx, fresh, password); err != nil {
		t.Fatal(err)
	}
	if _, err = fresh.Invoke(ctx, &mtproto.TLUpdatesGetState{Constructor: mtproto.CRC32_updates_getState}); err != nil {
		t.Fatalf("authed call after email login: %v", err)
	}
	pw, err = getPassword(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if pw.GetLoginEmailPattern().GetValue() != pattern || !pw.GetHasPassword() {
		t.Fatalf("password after email login pattern=%q has=%v", pw.GetLoginEmailPattern().GetValue(), pw.GetHasPassword())
	}
	if err = disableCloudPassword(ctx, fresh, password); err != nil {
		t.Fatal(err)
	}
	pw, err = getPassword(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if pw.GetHasPassword() || pw.GetLoginEmailPattern().GetValue() != pattern {
		t.Fatalf("after disable has=%v pattern=%q", pw.GetHasPassword(), pw.GetLoginEmailPattern().GetValue())
	}
}

func sendAuthCode(t *testing.T, ctx context.Context, c *Client, number string) *mtproto.TLAuthSentCode {
	t.Helper()
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		sent, err := c.Invoke(ctx, &mtproto.TLAuthSendCode{
			Constructor: mtproto.CRC32_auth_sendCode,
			PhoneNumber: number,
			ApiId:       c.cfg.APIID,
			ApiHash:     c.cfg.APIHash,
			Settings:    mtproto.MakeTLCodeSettings(&mtproto.CodeSettings{}).To_CodeSettings(),
		})
		if err == nil {
			code, ok := sent.(*mtproto.TLAuthSentCode)
			if !ok || code.GetPhoneCodeHash() == "" {
				t.Fatalf("auth.sendCode returned %T", sent)
			}
			return code
		}
		last = err
		wait, ok := emailFloodWait(err)
		if !ok {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(wait):
		}
	}
	t.Fatal(last)
	return nil
}

func loginPhoneAccount(t *testing.T, ctx context.Context, c *Client, phone, email, password string) (Self, error) {
	t.Helper()
	before := emailCodeCount(t, email)
	sent := sendAuthCode(t, ctx, c, phone)
	switch sent.GetType().GetPredicateName() {
	case mtproto.Predicate_auth_sentCodeTypeSms, mtproto.Predicate_auth_sentCodeTypeApp:
		return finishPhoneLogin(ctx, c, phone, sent.GetPhoneCodeHash(), "11111", "EmailLogin")
	case mtproto.Predicate_auth_sentCodeTypeEmailCode:
		code := waitEmailCode(t, email, before)
		return finishEmailLogin(ctx, c, phone, sent.GetPhoneCodeHash(), code, password)
	default:
		return Self{}, fmt.Errorf("auth.sendCode type %s", sent.GetType().GetPredicateName())
	}
}

func finishPhoneLogin(ctx context.Context, c *Client, phone, hash, code, firstName string) (Self, error) {
	signed, err := c.Invoke(ctx, &mtproto.TLAuthSignIn{
		Constructor:          mtproto.CRC32_auth_signIn_8d52a951,
		PhoneNumber:          phone,
		PhoneCodeHash:        hash,
		PhoneCode_FLAGSTRING: strFlag(code),
	})
	if err != nil {
		return Self{}, err
	}
	switch authz := signed.(type) {
	case *mtproto.TLAuthAuthorization:
		return selfFrom(authz.GetUser())
	case *mtproto.TLAuthAuthorizationSignUpRequired:
		created, err := c.Invoke(ctx, &mtproto.TLAuthSignUp{
			Constructor:   mtproto.CRC32_auth_signUp_aac7b717,
			PhoneNumber:   phone,
			PhoneCodeHash: hash,
			FirstName:     firstName,
		})
		if err != nil {
			return Self{}, err
		}
		auth, ok := created.(*mtproto.TLAuthAuthorization)
		if !ok {
			return Self{}, fmt.Errorf("auth.signUp returned %T", created)
		}
		return selfFrom(auth.GetUser())
	default:
		return Self{}, fmt.Errorf("auth.signIn returned %T", signed)
	}
}

func finishEmailLogin(ctx context.Context, c *Client, phone, hash, code, password string) (Self, error) {
	signed, err := signInEmail(ctx, c, phone, hash, mtproto.MakeTLEmailVerificationCode(&mtproto.EmailVerification{
		Code: code,
	}).To_EmailVerification())
	if err != nil {
		rpc, ok := asRPCError(err)
		if !ok || rpc.Message != "SESSION_PASSWORD_NEEDED" {
			return Self{}, err
		}
		return acceptCloudPassword(ctx, c, password)
	}
	authz, ok := signed.(*mtproto.TLAuthAuthorization)
	if !ok {
		return Self{}, fmt.Errorf("auth.signIn returned %T", signed)
	}
	return selfFrom(authz.GetUser())
}

func signInEmail(ctx context.Context, c *Client, phone, hash string, verification *mtproto.EmailVerification) (mtproto.TLObject, error) {
	return c.Invoke(ctx, &mtproto.TLAuthSignIn{
		Constructor:       mtproto.CRC32_auth_signIn_8d52a951,
		PhoneNumber:       phone,
		PhoneCodeHash:     hash,
		EmailVerification: verification,
	})
}

func acceptCloudPassword(ctx context.Context, c *Client, password string) (Self, error) {
	pw, err := getPassword(ctx, c)
	if err != nil {
		return Self{}, err
	}
	proof, err := checkPasswordProof(pw, password)
	if err != nil {
		return Self{}, err
	}
	authed, err := c.Invoke(ctx, &mtproto.TLAuthCheckPassword{
		Constructor: mtproto.CRC32_auth_checkPassword,
		Password:    proof,
	})
	if err != nil {
		return Self{}, err
	}
	authz, ok := authed.(*mtproto.TLAuthAuthorization)
	if !ok {
		return Self{}, fmt.Errorf("auth.checkPassword returned %T", authed)
	}
	return selfFrom(authz.GetUser())
}

func ensureLoginEmail(t *testing.T, ctx context.Context, c *Client, email, pattern string) error {
	t.Helper()
	pw, err := getPassword(ctx, c)
	if err != nil {
		return err
	}
	if pw.GetLoginEmailPattern().GetValue() == pattern {
		return nil
	}
	before := emailCodeCount(t, email)
	purpose := mtproto.MakeTLEmailVerifyPurposeLoginChange(&mtproto.EmailVerifyPurpose{}).To_EmailVerifyPurpose()
	sent, err := c.Invoke(ctx, &mtproto.TLAccountSendVerifyEmailCode{
		Constructor: mtproto.CRC32_account_sendVerifyEmailCode_98e037bb,
		Purpose:     purpose,
		Email:       email,
	})
	if err != nil {
		return err
	}
	mail, ok := sent.(*mtproto.TLAccountSentEmailCode)
	if !ok {
		return fmt.Errorf("sendVerifyEmailCode returned %T", sent)
	}
	if mail.GetEmailPattern() != pattern {
		return fmt.Errorf("sendVerifyEmailCode pattern %q", mail.GetEmailPattern())
	}
	code := waitEmailCode(t, email, before)
	verified, err := c.Invoke(ctx, &mtproto.TLAccountVerifyEmail32DA4CF{
		Constructor: mtproto.CRC32_account_verifyEmail32DA4CF,
		Purpose:     purpose,
		Verification: mtproto.MakeTLEmailVerificationCode(&mtproto.EmailVerification{
			Code: code,
		}).To_EmailVerification(),
	})
	if err != nil {
		return err
	}
	done, ok := verified.(*mtproto.TLAccountEmailVerified)
	if !ok {
		return fmt.Errorf("verifyEmail returned %T", verified)
	}
	if done.GetEmail() != email {
		return fmt.Errorf("verifyEmail stored %q", done.GetEmail())
	}
	return nil
}
