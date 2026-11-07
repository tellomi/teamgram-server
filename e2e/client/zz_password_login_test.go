package e2e

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestCloudPasswordReloginAndDisable walks the two client bugs:
// logout on the same permanent key must still return an SRP challenge, and
// disabling 2FA uses passwordKdfAlgoUnknown plus an empty hash.
func TestCloudPasswordReloginAndDisable(t *testing.T) {
	cfg := requireE2E(t)
	phone, err := cfg.Phone("password")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client, err := Dial(ctx, cfg, ProfileAndroid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	// The display name is not the cloud password. "Password" is in the
	// signup lexicon, so a new account cannot use it.
	if _, err = client.Login(ctx, phone, "Ada"); err != nil {
		t.Fatal(err)
	}
	if err = setCloudPassword(ctx, client, "secret"); err != nil {
		t.Fatal(err)
	}

	saved, ok := client.Session()
	if !ok {
		t.Fatal("missing auth key before logout")
	}
	if _, err = client.Invoke(ctx, &mtproto.TLAuthLogOut{Constructor: mtproto.CRC32_auth_logOut}); err != nil {
		t.Fatal(err)
	}
	client.Close()

	// Same permanent key, new connection. This is the Android spinner case.
	client, err = DialSession(ctx, cfg, ProfileAndroid, saved)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if err = expectPasswordNeeded(ctx, client, phone); err != nil {
		t.Fatal(err)
	}
	pw, err := getPassword(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	if !pw.GetHasPassword() || pw.GetCurrentAlgo() == nil || len(pw.GetSrp_B()) != 256 || pw.GetSrpId().GetValue() == 0 {
		t.Fatalf("getPassword has=%v algo=%v b=%d id=%d", pw.GetHasPassword(), pw.GetCurrentAlgo() != nil, len(pw.GetSrp_B()), pw.GetSrpId().GetValue())
	}
	proof, err := checkPasswordProof(pw, "secret")
	if err != nil {
		t.Fatal(err)
	}
	authed, err := client.Invoke(ctx, &mtproto.TLAuthCheckPassword{
		Constructor: mtproto.CRC32_auth_checkPassword,
		Password:    proof,
	})
	if err != nil {
		t.Fatal(err)
	}
	authz, ok := authed.(*mtproto.TLAuthAuthorization)
	if !ok || authz.GetUser().GetId() == 0 {
		t.Fatalf("checkPassword returned %T", authed)
	}
	if _, err = client.Invoke(ctx, &mtproto.TLUpdatesGetState{Constructor: mtproto.CRC32_updates_getState}); err != nil {
		t.Fatalf("authed call after checkPassword: %v", err)
	}

	if err = disableCloudPassword(ctx, client, "secret"); err != nil {
		t.Fatal(err)
	}
	pw, err = getPassword(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	if pw.GetHasPassword() || pw.GetCurrentAlgo() != nil || len(pw.GetSrp_B()) != 0 {
		t.Fatalf("password still set: has=%v", pw.GetHasPassword())
	}

	if _, err = client.Invoke(ctx, &mtproto.TLAuthLogOut{Constructor: mtproto.CRC32_auth_logOut}); err != nil {
		t.Fatal(err)
	}
	client.Close()
	client, err = DialSession(ctx, cfg, ProfileAndroid, saved)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	self, err := client.Login(ctx, phone, "Ada")
	if err != nil {
		t.Fatalf("login without password: %v", err)
	}
	if self.ID == 0 {
		t.Fatal("login without password returned no user")
	}
	if _, err = client.Invoke(ctx, &mtproto.TLUpdatesGetState{Constructor: mtproto.CRC32_updates_getState}); err != nil {
		t.Fatalf("authed call without password: %v", err)
	}
}

func expectPasswordNeeded(ctx context.Context, c *Client, phone Phone) error {
	sent, err := c.Invoke(ctx, &mtproto.TLAuthSendCode{
		Constructor: mtproto.CRC32_auth_sendCode,
		PhoneNumber: phone.Number,
		ApiId:       c.cfg.APIID,
		ApiHash:     c.cfg.APIHash,
		Settings:    mtproto.MakeTLCodeSettings(&mtproto.CodeSettings{}).To_CodeSettings(),
	})
	if err != nil {
		return fmt.Errorf("auth.sendCode: %w", err)
	}
	code, ok := sent.(*mtproto.TLAuthSentCode)
	if !ok || code.GetPhoneCodeHash() == "" {
		return fmt.Errorf("auth.sendCode returned %T", sent)
	}
	_, err = c.Invoke(ctx, &mtproto.TLAuthSignIn{
		Constructor:          mtproto.CRC32_auth_signIn_8d52a951,
		PhoneNumber:          phone.Number,
		PhoneCodeHash:        code.GetPhoneCodeHash(),
		PhoneCode_FLAGSTRING: strFlag(phone.Code),
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "SESSION_PASSWORD_NEEDED" {
		return fmt.Errorf("auth.signIn: %w", err)
	}
	return nil
}

func getPassword(ctx context.Context, c *Client) (*mtproto.TLAccountPassword, error) {
	obj, err := c.Invoke(ctx, &mtproto.TLAccountGetPassword{Constructor: mtproto.CRC32_account_getPassword})
	if err != nil {
		return nil, err
	}
	pw, ok := obj.(*mtproto.TLAccountPassword)
	if !ok {
		return nil, fmt.Errorf("account.getPassword returned %T", obj)
	}
	return pw, nil
}

func setCloudPassword(ctx context.Context, c *Client, pass string) error {
	pw, err := getPassword(ctx, c)
	if err != nil {
		return err
	}
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{0x11}, 32)...)
	salt2 := append([]byte{}, pw.GetNewAlgo().GetSalt2()...)
	obj, err := c.Invoke(ctx, &mtproto.TLAccountUpdatePasswordSettings{
		Constructor: mtproto.CRC32_account_updatePasswordSettings,
		Password:    mtproto.MakeTLInputCheckPasswordEmpty(nil).To_InputCheckPasswordSRP(),
		NewSettings: mtproto.MakeTLAccountPasswordInputSettings(&mtproto.Account_PasswordInputSettings{
			NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
				Salt1: salt1,
				Salt2: salt2,
				G:     pw.GetNewAlgo().GetG(),
				P:     append([]byte{}, pw.GetNewAlgo().GetP()...),
			}).To_PasswordKdfAlgo(),
			NewPasswordHash: Verifier([]byte(pass), salt1, salt2),
			Hint:            &wrapperspb.StringValue{Value: "hint"},
		}).To_Account_PasswordInputSettings(),
	})
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	if _, ok := obj.(*mtproto.TLBoolTrue); !ok {
		return fmt.Errorf("set password returned %T", obj)
	}
	return nil
}

func disableCloudPassword(ctx context.Context, c *Client, pass string) error {
	pw, err := getPassword(ctx, c)
	if err != nil {
		return err
	}
	proof, err := checkPasswordProof(pw, pass)
	if err != nil {
		return err
	}
	// Android: unknown algorithm, empty hash, empty hint.
	obj, err := c.Invoke(ctx, &mtproto.TLAccountUpdatePasswordSettings{
		Constructor: mtproto.CRC32_account_updatePasswordSettings,
		Password:    proof,
		NewSettings: mtproto.MakeTLAccountPasswordInputSettings(&mtproto.Account_PasswordInputSettings{
			NewAlgo:         mtproto.MakeTLPasswordKdfAlgoUnknown(nil).To_PasswordKdfAlgo(),
			NewPasswordHash: []byte{},
			Hint:            &wrapperspb.StringValue{Value: ""},
		}).To_Account_PasswordInputSettings(),
	})
	if err != nil {
		return fmt.Errorf("disable password: %w", err)
	}
	if _, ok := obj.(*mtproto.TLBoolTrue); !ok {
		return fmt.Errorf("disable password returned %T", obj)
	}
	return nil
}

func TestPasswordSettingsRoundTrip(t *testing.T) {
	salt1 := bytes.Repeat([]byte{1}, 40)
	salt2 := bytes.Repeat([]byte{2}, 16)
	hash := bytes.Repeat([]byte{3}, 256)
	req := &mtproto.TLAccountUpdatePasswordSettings{
		Constructor: mtproto.CRC32_account_updatePasswordSettings,
		Password:    mtproto.MakeTLInputCheckPasswordEmpty(nil).To_InputCheckPasswordSRP(),
		NewSettings: mtproto.MakeTLAccountPasswordInputSettings(&mtproto.Account_PasswordInputSettings{
			NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
				Salt1: salt1,
				Salt2: salt2,
				G:     3,
				P:     append([]byte{}, Prime...),
			}).To_PasswordKdfAlgo(),
			NewPasswordHash: hash,
			Hint:            &wrapperspb.StringValue{Value: "hint"},
		}).To_Account_PasswordInputSettings(),
	}
	raw, err := encodeTL(req)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := decodeTL(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := obj.(*mtproto.TLAccountUpdatePasswordSettings)
	if !ok || got.GetNewSettings().GetNewAlgo().GetPredicateName() != mtproto.Predicate_passwordKdfAlgoModPow {
		t.Fatalf("round trip %T algo %s", obj, got.GetNewSettings().GetNewAlgo().GetPredicateName())
	}
	if !bytes.Equal(got.GetNewSettings().GetNewPasswordHash(), hash) || got.GetNewSettings().GetHint().GetValue() != "hint" {
		t.Fatalf("settings %+v", got.GetNewSettings())
	}

	disable := &mtproto.TLAccountUpdatePasswordSettings{
		Constructor: mtproto.CRC32_account_updatePasswordSettings,
		Password:    mtproto.MakeTLInputCheckPasswordEmpty(nil).To_InputCheckPasswordSRP(),
		NewSettings: mtproto.MakeTLAccountPasswordInputSettings(&mtproto.Account_PasswordInputSettings{
			NewAlgo:         mtproto.MakeTLPasswordKdfAlgoUnknown(nil).To_PasswordKdfAlgo(),
			NewPasswordHash: []byte{},
			Hint:            &wrapperspb.StringValue{Value: ""},
		}).To_Account_PasswordInputSettings(),
	}
	raw, err = encodeTL(disable)
	if err != nil {
		t.Fatal(err)
	}
	obj, err = decodeTL(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, ok = obj.(*mtproto.TLAccountUpdatePasswordSettings)
	if !ok || got.GetNewSettings().GetNewAlgo().GetPredicateName() != mtproto.Predicate_passwordKdfAlgoUnknown || len(got.GetNewSettings().GetNewPasswordHash()) != 0 {
		t.Fatalf("disable round trip %T %+v", obj, got.GetNewSettings())
	}
}

func checkPasswordProof(pw *mtproto.TLAccountPassword, pass string) (*mtproto.InputCheckPasswordSRP, error) {
	a := bytes.Repeat([]byte{0x3c}, 256)
	A, M1, err := ClientProof([]byte(pass), pw.GetCurrentAlgo().GetSalt1(), pw.GetCurrentAlgo().GetSalt2(), a, pw.GetSrp_B())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{
		SrpId: pw.GetSrpId().GetValue(),
		A:     A,
		M1:    M1,
	}).To_InputCheckPasswordSRP(), nil
}
