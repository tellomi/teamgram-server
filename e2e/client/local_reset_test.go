package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	gotdmt "github.com/gotd/td/mtproto"
	"github.com/teamgram/proto/mtproto"
)

// TestLocalAgedResetDropsOtherSessions ages only the synthetic user created
// in this test, and only when the MTProto target is loopback. It checks the
// fresh 406 on both devices, then a single reset and reset-all after 25h,
// including an offline reconnect. The UPDATE stays inside the local mysql
// container. It does not read a production credential and does not FLUSHALL.
func TestLocalAgedResetDropsOtherSessions(t *testing.T) {
	cfg := requireE2E(t)
	requireLocalReset(t, cfg)
	phone, err := cfg.Phone("aged")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	ios, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ios.Close)
	self, err := ios.Login(ctx, phone, "Aged")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = ios.State(ctx); err != nil {
		t.Fatal(err)
	}

	android, err := Dial(ctx, cfg, ProfileAndroid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(android.Close)
	if _, err = android.Login(ctx, phone, "Aged"); err != nil {
		t.Fatal(err)
	}

	iosHashes := hashesForDevice(listAuthorizations(t, ctx, android), ProfileIOS.DeviceModel)
	if len(iosHashes) != 1 {
		t.Fatalf("iOS rows = %d", len(iosHashes))
	}
	var iosHash int64
	for hash := range iosHashes {
		iosHash = hash
	}

	_, err = android.Invoke(ctx, &mtproto.TLAuthResetAuthorizations{Constructor: mtproto.CRC32_auth_resetAuthorizations})
	assertRPC(t, err, 406, "FRESH_RESET_AUTHORISATION_FORBIDDEN")
	_, err = android.Invoke(ctx, &mtproto.TLAccountResetAuthorization{
		Constructor: mtproto.CRC32_account_resetAuthorization,
		Hash:        iosHash,
	})
	assertRPC(t, err, 406, "FRESH_RESET_AUTHORISATION_FORBIDDEN")
	_, err = ios.Invoke(ctx, &mtproto.TLAuthResetAuthorizations{Constructor: mtproto.CRC32_auth_resetAuthorizations})
	assertRPC(t, err, 406, "FRESH_RESET_AUTHORISATION_FORBIDDEN")
	_, err = ios.Invoke(ctx, &mtproto.TLAccountResetAuthorization{
		Constructor: mtproto.CRC32_account_resetAuthorization,
		Hash:        1,
	})
	assertRPC(t, err, 406, "FRESH_RESET_AUTHORISATION_FORBIDDEN")
	if _, _, _, err = ios.State(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = android.State(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := hashesForDevice(listAuthorizations(t, ctx, android), ProfileIOS.DeviceModel)[iosHash]; !ok {
		t.Fatal("refused reset removed the iOS row")
	}

	ageLocalUser(t, ctx, self.ID)
	if _, err = android.Invoke(ctx, &mtproto.TLAccountResetAuthorization{
		Constructor: mtproto.CRC32_account_resetAuthorization,
		Hash:        iosHash,
	}); err != nil {
		t.Fatalf("aged single reset: %v", err)
	}
	expectLoggedOut(t, ctx, ios)
	iosSession, ok := ios.Session()
	if !ok {
		t.Fatal("missing iOS session")
	}
	ios.Close()
	expectReconnectLoggedOut(t, ctx, cfg, ProfileIOS, iosSession)
	if _, ok := hashesForDevice(listAuthorizations(t, ctx, android), ProfileIOS.DeviceModel)[iosHash]; ok {
		t.Fatal("revoked iOS row is still listed")
	}
	if _, _, _, err = android.State(ctx); err != nil {
		t.Fatal(err)
	}

	pixelProfile := ProfileIOS
	pixelProfile.DeviceModel = "Pixel 9"
	pixel, err := Dial(ctx, cfg, pixelProfile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pixel.Login(ctx, phone, "Aged"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = pixel.State(ctx); err != nil {
		t.Fatal(err)
	}
	pixelHashes := hashesForDevice(listAuthorizations(t, ctx, android), "Pixel 9")
	if len(pixelHashes) != 1 {
		t.Fatalf("pixel rows = %d", len(pixelHashes))
	}
	var pixelHash int64
	for hash := range pixelHashes {
		pixelHash = hash
	}
	pixelSession, ok := pixel.Session()
	if !ok {
		t.Fatal("missing offline session")
	}
	pixel.Close()

	ageLocalUser(t, ctx, self.ID)
	if _, err = android.Invoke(ctx, &mtproto.TLAccountResetAuthorization{
		Constructor: mtproto.CRC32_account_resetAuthorization,
		Hash:        pixelHash,
	}); err != nil {
		t.Fatalf("aged offline reset: %v", err)
	}
	expectReconnectLoggedOut(t, ctx, cfg, pixelProfile, pixelSession)
	if _, ok := hashesForDevice(listAuthorizations(t, ctx, android), "Pixel 9")[pixelHash]; ok {
		t.Fatal("offline row stayed in the list")
	}

	fresh, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Close)
	if _, err = fresh.Login(ctx, phone, "Aged"); err != nil {
		t.Fatal(err)
	}
	_, err = fresh.Invoke(ctx, &mtproto.TLAuthResetAuthorizations{Constructor: mtproto.CRC32_auth_resetAuthorizations})
	assertRPC(t, err, 406, "FRESH_RESET_AUTHORISATION_FORBIDDEN")
	_, err = fresh.Invoke(ctx, &mtproto.TLAccountResetAuthorization{
		Constructor: mtproto.CRC32_account_resetAuthorization,
		Hash:        1,
	})
	assertRPC(t, err, 406, "FRESH_RESET_AUTHORISATION_FORBIDDEN")
	if _, _, _, err = android.State(ctx); err != nil {
		t.Fatal(err)
	}
	if len(hashesForDevice(listAuthorizations(t, ctx, android), ProfileIOS.DeviceModel)) == 0 {
		t.Fatal("fresh refusal removed the new iOS row")
	}

	freshSession, ok := fresh.Session()
	if !ok {
		t.Fatal("missing fresh session")
	}
	if _, err = android.Invoke(ctx, &mtproto.TLAuthResetAuthorizations{Constructor: mtproto.CRC32_auth_resetAuthorizations}); err != nil {
		t.Fatalf("aged reset all: %v", err)
	}
	expectLoggedOut(t, ctx, fresh)
	fresh.Close()
	expectReconnectLoggedOut(t, ctx, cfg, ProfileIOS, freshSession)
	if otherDevices(listAuthorizations(t, ctx, android)) != 0 {
		t.Fatal("reset all left another device")
	}
	if _, _, _, err = android.State(ctx); err != nil {
		t.Fatal(err)
	}
}

func requireLocalReset(t *testing.T, cfg Config) {
	t.Helper()
	if os.Getenv("TELLOMI_E2E_LOCAL_RESET") != "1" {
		t.Skip("set TELLOMI_E2E_LOCAL_RESET=1 to age a synthetic session on the local compose MySQL")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		t.Fatalf("addr is not host:port")
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatalf("refusing to age sessions through a non-loopback host")
	}
}

func ageLocalUser(t *testing.T, ctx context.Context, userID int64) {
	t.Helper()
	if userID <= 0 {
		t.Fatal("missing synthetic user")
	}
	query := fmt.Sprintf("UPDATE auth_users SET date_created = UNIX_TIMESTAMP() - 90000 WHERE user_id = %d AND deleted = 0; SELECT ROW_COUNT();", userID)
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", "mysql", "sh", "-c", `MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --batch --skip-column-names --raw -uroot teamgram`)
	cmd.Stdin = strings.NewReader(query)
	out, err := cmd.CombinedOutput()
	redacted := redactCommandOutput(string(out))
	if err != nil {
		t.Fatalf("local age update failed: %v\n%s", err, redacted)
	}
	fields := strings.Fields(redacted)
	if len(fields) == 0 || fields[len(fields)-1] == "0" {
		t.Fatalf("local age update changed %q rows", redacted)
	}
}

func redactCommandOutput(out string) string {
	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "mysql_pwd") || strings.Contains(lower, "password") {
			b.WriteString("[redacted]\n")
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func expectLoggedOut(t *testing.T, ctx context.Context, c *Client) {
	t.Helper()
	_, _, _, err := c.State(ctx)
	assertRPC(t, err, 401, "AUTH_KEY_UNREGISTERED")
}

func expectReconnectLoggedOut(t *testing.T, ctx context.Context, cfg Config, profile Profile, sess gotdmt.Session) {
	t.Helper()
	again, err := DialSession(ctx, cfg, profile, sess)
	if err != nil {
		return
	}
	t.Cleanup(again.Close)
	expectLoggedOut(t, ctx, again)
}

func otherDevices(rows []*mtproto.Authorization) int {
	n := 0
	for _, row := range rows {
		if row != nil && !row.GetCurrent() {
			n++
		}
	}
	return n
}
