package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestQRLoginNoticeWithoutPassword is a finished QR login with no cloud
// password. The 777000 message names the new device, not the scanner.
// Replaying the token or presenting a bad token does not add another notice.
func TestQRLoginNoticeWithoutPassword(t *testing.T) {
	cfg := requireE2E(t)
	phone, err := cfg.Phone("qr")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	scanner, err := Dial(ctx, cfg, ProfileAndroid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(scanner.Close)
	scannerAt := time.Now()
	if _, err = scanner.Login(ctx, phone, "QR"); err != nil {
		t.Fatal(err)
	}
	waitSecurityNotice(t, ctx, scanner, ProfileAndroid.DeviceModel, scannerAt)

	fresh, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Close)
	token, err := exportLoginToken(ctx, fresh, cfg)
	if err != nil {
		t.Fatal(err)
	}
	before := countSecurityNotices(t, ctx, scanner)
	acceptedAt := time.Now()
	if _, err = scanner.Invoke(ctx, &mtproto.TLAuthAcceptLoginToken{
		Constructor: mtproto.CRC32_auth_acceptLoginToken,
		Token:       token,
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	signed, err := fresh.Invoke(ctx, &mtproto.TLAuthExportLoginToken{
		Constructor: mtproto.CRC32_auth_exportLoginToken,
		ApiId:       cfg.APIID,
		ApiHash:     cfg.APIHash,
	})
	if err != nil {
		t.Fatalf("export after accept: %v", err)
	}
	if _, ok := signed.(*mtproto.TLAuthLoginTokenSuccess); !ok {
		t.Fatalf("export after accept returned %T", signed)
	}
	text := waitSecurityNotice(t, ctx, scanner, ProfileIOS.DeviceModel, acceptedAt)
	assertSecurityNotice(t, text, ProfileIOS.DeviceModel, acceptedAt, time.Now())
	if strings.Contains(text, ProfileAndroid.DeviceModel) {
		t.Fatal("notice named the scanner instead of the new device")
	}
	if _, _, _, err = fresh.State(ctx); err != nil {
		t.Fatalf("new QR session: %v", err)
	}
	stable := countSecurityNotices(t, ctx, scanner)
	if stable <= before {
		t.Fatalf("security notices %d, want a new one after %d", stable, before)
	}

	_, err = scanner.Invoke(ctx, &mtproto.TLAuthAcceptLoginToken{
		Constructor: mtproto.CRC32_auth_acceptLoginToken,
		Token:       token,
	})
	if err == nil {
		t.Fatal("replayed token was accepted")
	}
	if _, err = scanner.Invoke(ctx, &mtproto.TLAuthAcceptLoginToken{
		Constructor: mtproto.CRC32_auth_acceptLoginToken,
		Token:       make([]byte, 24),
	}); err == nil {
		t.Fatal("empty token was accepted")
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if got := countSecurityNotices(t, ctx, scanner); got != stable {
			t.Fatalf("security notices %d, want %d after replay and a bad token", got, stable)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

// TestQRLoginNoticeWithPassword accepts a scan while a cloud password is
// set. The 777000 message waits until auth.checkPassword succeeds.
func TestQRLoginNoticeWithPassword(t *testing.T) {
	cfg := requireE2E(t)
	phone, err := cfg.Phone("qrpass")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	scanner, err := Dial(ctx, cfg, ProfileAndroid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(scanner.Close)
	scannerAt := time.Now()
	if _, err = scanner.Login(ctx, phone, "QRPass"); err != nil {
		t.Fatal(err)
	}
	waitSecurityNotice(t, ctx, scanner, ProfileAndroid.DeviceModel, scannerAt)
	if err = setCloudPassword(ctx, scanner, "secret"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		off, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = disableCloudPassword(off, scanner, "secret")
	})

	fresh, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Close)
	token, err := exportLoginToken(ctx, fresh, cfg)
	if err != nil {
		t.Fatal(err)
	}
	before := countSecurityNotices(t, ctx, scanner)
	if _, err = scanner.Invoke(ctx, &mtproto.TLAuthAcceptLoginToken{
		Constructor: mtproto.CRC32_auth_acceptLoginToken,
		Token:       token,
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	_, err = fresh.Invoke(ctx, &mtproto.TLAuthExportLoginToken{
		Constructor: mtproto.CRC32_auth_exportLoginToken,
		ApiId:       cfg.APIID,
		ApiHash:     cfg.APIHash,
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "SESSION_PASSWORD_NEEDED" {
		t.Fatalf("export after a password scan: %v", err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if got := countSecurityNotices(t, ctx, scanner); got != before {
			t.Fatalf("security notices %d, want %d before the password proof", got, before)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}

	pw, err := getPassword(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := checkPasswordProof(pw, "secret")
	if err != nil {
		t.Fatal(err)
	}
	checkedAt := time.Now()
	if _, err = fresh.Invoke(ctx, &mtproto.TLAuthCheckPassword{
		Constructor: mtproto.CRC32_auth_checkPassword,
		Password:    proof,
	}); err != nil {
		t.Fatalf("checkPassword: %v", err)
	}
	text := waitSecurityNotice(t, ctx, scanner, ProfileIOS.DeviceModel, checkedAt)
	assertSecurityNotice(t, text, ProfileIOS.DeviceModel, checkedAt, time.Now())
	if strings.Contains(text, ProfileAndroid.DeviceModel) {
		t.Fatal("notice named the scanner instead of the new device")
	}
	if got := countSecurityNotices(t, ctx, scanner); got != before+1 {
		t.Fatalf("security notices %d, want %d", got, before+1)
	}
	if err = disableCloudPassword(ctx, fresh, "secret"); err != nil {
		t.Fatal(err)
	}
}

func exportLoginToken(ctx context.Context, c *Client, cfg Config) ([]byte, error) {
	res, err := c.Invoke(ctx, &mtproto.TLAuthExportLoginToken{
		Constructor: mtproto.CRC32_auth_exportLoginToken,
		ApiId:       cfg.APIID,
		ApiHash:     cfg.APIHash,
	})
	if err != nil {
		return nil, err
	}
	tok, ok := res.(*mtproto.TLAuthLoginToken)
	if !ok || tok.GetData2() == nil || len(tok.GetData2().GetToken()) != 24 {
		return nil, errString("export did not return a login token")
	}
	return append([]byte(nil), tok.GetData2().GetToken()...), nil
}
