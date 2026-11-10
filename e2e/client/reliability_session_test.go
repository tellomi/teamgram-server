//go:build reliability

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestReliabilitySessionRevocation checks the official reset rules on an
// isolated stack. A session younger than 24 hours is refused. After that
// window, account.resetAuthorization and auth.resetAuthorizations drop the
// other auth key before the RPC returns: the next request is 401
// AUTH_KEY_UNREGISTERED, including a reconnect with the saved key.
func TestReliabilitySessionRevocation(t *testing.T) {
	// Own users only. Runs alongside multi-device sync on the shared stack.
	t.Parallel()
	s := useReliabilityStack(t)
	phone := reliabilityPhone("0301")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	ios, iosSelf := s.login(t, ProfileIOS, phone, "Revoke")
	android, _ := s.login(t, ProfileAndroid, phone, "Revoke")

	iosHashes := hashesForDevice(listAuthorizations(t, ctx, android), ProfileIOS.DeviceModel)
	if len(iosHashes) != 1 {
		t.Fatalf("iOS rows = %d", len(iosHashes))
	}
	var iosHash int64
	for hash := range iosHashes {
		iosHash = hash
	}

	_, err := android.Invoke(ctx, &mtproto.TLAuthResetAuthorizations{Constructor: mtproto.CRC32_auth_resetAuthorizations})
	assertRPC(t, err, 406, "FRESH_RESET_AUTHORISATION_FORBIDDEN")
	_, err = android.Invoke(ctx, &mtproto.TLAccountResetAuthorization{
		Constructor: mtproto.CRC32_account_resetAuthorization,
		Hash:        iosHash,
	})
	assertRPC(t, err, 406, "FRESH_RESET_AUTHORISATION_FORBIDDEN")
	_, err = android.Invoke(ctx, &mtproto.TLAccountResetAuthorization{
		Constructor: mtproto.CRC32_account_resetAuthorization,
		Hash:        0,
	})
	assertRPC(t, err, 400, "HASH_INVALID")
	callState(t, ios)
	callState(t, android)

	s.execSQL(t, "UPDATE auth_users SET date_created = UNIX_TIMESTAMP() - 90000 WHERE user_id = ? AND deleted = 0", iosSelf.ID)
	if _, err = android.Invoke(ctx, &mtproto.TLAccountResetAuthorization{
		Constructor: mtproto.CRC32_account_resetAuthorization,
		Hash:        iosHash,
	}); err != nil {
		t.Fatalf("aged account.resetAuthorization: %v", err)
	}
	_, _, _, err = ios.State(ctx)
	assertRPC(t, err, 401, "AUTH_KEY_UNREGISTERED")
	iosSession, ok := ios.Session()
	if !ok {
		t.Fatal("missing iOS session")
	}
	ios.Close()
	expectReconnectLoggedOut(t, ctx, s.cfg, ProfileIOS, iosSession)
	if _, ok := hashesForDevice(listAuthorizations(t, ctx, android), ProfileIOS.DeviceModel)[iosHash]; ok {
		t.Fatal("revoked iOS row is still listed")
	}
	callState(t, android)

	pixelProfile := ProfileIOS
	pixelProfile.DeviceModel = "Pixel 9"
	pixel, _ := s.login(t, pixelProfile, phone, "Revoke")
	pixelHashes := hashesForDevice(listAuthorizations(t, ctx, android), "Pixel 9")
	if len(pixelHashes) != 1 {
		t.Fatalf("pixel rows = %d", len(pixelHashes))
	}
	var pixelHash int64
	for hash := range pixelHashes {
		pixelHash = hash
	}
	s.execSQL(t, "UPDATE auth_users SET date_created = UNIX_TIMESTAMP() - 90000 WHERE user_id = ? AND deleted = 0", iosSelf.ID)
	if _, err = android.Invoke(ctx, &mtproto.TLAuthResetAuthorizations{Constructor: mtproto.CRC32_auth_resetAuthorizations}); err != nil {
		t.Fatalf("aged auth.resetAuthorizations: %v", err)
	}
	_, _, _, err = pixel.State(ctx)
	assertRPC(t, err, 401, "AUTH_KEY_UNREGISTERED")
	pixelSession, ok := pixel.Session()
	if !ok {
		t.Fatal("missing pixel session")
	}
	pixel.Close()
	expectReconnectLoggedOut(t, ctx, s.cfg, pixelProfile, pixelSession)
	if otherDevices(listAuthorizations(t, ctx, android)) != 0 {
		t.Fatal("reset all left another device")
	}
	callState(t, android)
	if _, ok := hashesForDevice(listAuthorizations(t, ctx, android), "Pixel 9")[pixelHash]; ok {
		t.Fatal("reset-all row is still listed")
	}
}
