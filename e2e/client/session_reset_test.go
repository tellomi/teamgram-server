package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestFreshLoginCannotResetOlderSession is two synthetic MTProto sessions of
// one test phone. The Android login is minutes old, so both reset RPCs must
// return 406 FRESH_RESET_AUTHORISATION_FORBIDDEN and the iOS row must stay.
// The 24h boundary itself is covered by the authsession clock tests. This
// test does not change date_created.
func TestFreshLoginCannotResetOlderSession(t *testing.T) {
	cfg := requireE2E(t)
	phone, err := cfg.Phone("reset_pair")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	ios, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatalf("dial ios: %v", err)
	}
	t.Cleanup(ios.Close)
	if _, err = ios.Login(ctx, phone, "Reset"); err != nil {
		t.Fatalf("ios login: %v", err)
	}
	if _, _, _, err = ios.State(ctx); err != nil {
		t.Fatalf("ios getState: %v", err)
	}

	android, err := Dial(ctx, cfg, ProfileAndroid)
	if err != nil {
		t.Fatalf("dial android: %v", err)
	}
	t.Cleanup(android.Close)
	if _, err = android.Login(ctx, phone, "Reset"); err != nil {
		t.Fatalf("android login: %v", err)
	}

	before := listAuthorizations(t, ctx, android)
	iosHashes := hashesForDevice(before, "iPhone 16")
	if len(iosHashes) == 0 {
		t.Fatalf("android device list has no iOS session: %d rows", len(before))
	}
	var iosHash int64
	for hash := range iosHashes {
		iosHash = hash
		break
	}

	_, err = android.Invoke(ctx, &mtproto.TLAuthResetAuthorizations{
		Constructor: mtproto.CRC32_auth_resetAuthorizations,
	})
	assertRPC(t, err, 406, "FRESH_RESET_AUTHORISATION_FORBIDDEN")

	_, err = android.Invoke(ctx, &mtproto.TLAccountResetAuthorization{
		Constructor: mtproto.CRC32_account_resetAuthorization,
		Hash:        iosHash,
	})
	assertRPC(t, err, 406, "FRESH_RESET_AUTHORISATION_FORBIDDEN")

	_, err = android.Invoke(ctx, &mtproto.TLAccountResetAuthorization{
		Constructor: mtproto.CRC32_account_resetAuthorization,
		Hash:        1,
	})
	assertRPC(t, err, 406, "FRESH_RESET_AUTHORISATION_FORBIDDEN")

	_, err = android.Invoke(ctx, &mtproto.TLAccountResetAuthorization{
		Constructor: mtproto.CRC32_account_resetAuthorization,
		Hash:        0,
	})
	assertRPC(t, err, 400, "HASH_INVALID")

	after := hashesForDevice(listAuthorizations(t, ctx, android), "iPhone 16")
	for hash := range iosHashes {
		if _, ok := after[hash]; !ok {
			t.Fatalf("iOS session %d left the device list after a refused reset", hash)
		}
	}
	if _, _, _, err = ios.State(ctx); err != nil {
		t.Fatalf("iOS session stopped answering: %v", err)
	}
}

func listAuthorizations(t *testing.T, ctx context.Context, c *Client) []*mtproto.Authorization {
	t.Helper()
	res, err := c.Invoke(ctx, &mtproto.TLAccountGetAuthorizations{
		Constructor: mtproto.CRC32_account_getAuthorizations,
	})
	if err != nil {
		t.Fatal(err)
	}
	list, ok := res.(*mtproto.TLAccountAuthorizations)
	if !ok || list.GetData2() == nil {
		t.Fatalf("account.getAuthorizations returned %T", res)
	}
	return list.GetData2().GetAuthorizations()
}

func hashesForDevice(rows []*mtproto.Authorization, device string) map[int64]struct{} {
	out := map[int64]struct{}{}
	for _, row := range rows {
		if row == nil || row.GetCurrent() || row.GetDeviceModel() != device || row.GetHash() == 0 {
			continue
		}
		out[row.GetHash()] = struct{}{}
	}
	return out
}

func assertRPC(t *testing.T, err error, code int, message string) {
	t.Helper()
	rpc, ok := asRPCError(err)
	if !ok {
		t.Fatalf("got %v, want rpc %d %s", err, code, message)
	}
	if rpc.Code != code || rpc.Message != message {
		t.Fatalf("rpc %d %s, want %d %s", rpc.Code, rpc.Message, code, message)
	}
}
