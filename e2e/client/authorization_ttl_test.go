package e2e

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// TestSetAuthorizationTTLAcceptsOneWeek is the session-timeout screen.
// Android and iOS both offer 7 days. A value outside the official set is
// TTL_DAYS_INVALID and is not stored.
func TestSetAuthorizationTTLAcceptsOneWeek(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileAndroid, "ttl7", "TTL")
	ctx, cancel := ctx()
	defer cancel()

	_, err := c.Invoke(ctx, &mtproto.TLAccountSetAuthorizationTTL{
		Constructor:          mtproto.CRC32_account_setAuthorizationTTL,
		AuthorizationTtlDays: 6,
	})
	rpc, ok := asRPCError(err)
	if err == nil || !ok || !strings.Contains(rpc.Message, "TTL_DAYS_INVALID") {
		t.Fatalf("6 days: %v", err)
	}

	stored := invokeOK(t, c, &mtproto.TLAccountSetAuthorizationTTL{
		Constructor:          mtproto.CRC32_account_setAuthorizationTTL,
		AuthorizationTtlDays: 7,
	})
	if _, yes := stored.(*mtproto.TLBoolTrue); !yes {
		t.Fatalf("set 7 days: %T", stored)
	}

	got := invokeOK(t, c, &mtproto.TLAccountGetAccountTTL{
		Constructor: mtproto.CRC32_account_getAccountTTL,
	})
	if days := accountTTLDays(got); days != 7 {
		t.Fatalf("account.getAccountTTL = %d (%T)", days, got)
	}
}

func accountTTLDays(obj mtproto.TLObject) int32 {
	switch v := obj.(type) {
	case *mtproto.AccountDaysTTL:
		return v.GetDays()
	case *mtproto.TLAccountDaysTTL:
		return v.GetDays()
	default:
		return -1
	}
}
