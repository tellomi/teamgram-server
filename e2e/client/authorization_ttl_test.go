package e2e

import (
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// TestSetAuthorizationTTLAcceptsOneWeek is the session-timeout screen.
// Android sends 7 and Desktop sends 7. The stored value is
// account.getAuthorizations.authorization_ttl_days. account.getAccountTTL is
// the self-destruct timer and must stay unchanged. The RPC must not log the
// session out or drop another authorization.
func TestSetAuthorizationTTLAcceptsOneWeek(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileAndroid, "ttl7", "TTL")
	ctx, cancel := ctx()
	defer cancel()

	accountBefore := accountTTLDays(invokeOK(t, c, &mtproto.TLAccountGetAccountTTL{
		Constructor: mtproto.CRC32_account_getAccountTTL,
	}))
	if accountBefore < 1 {
		t.Fatalf("account.getAccountTTL = %d", accountBefore)
	}
	_, beforeRows := readAuthorizations(t, c)
	beforeKeys := authorizationKeys(beforeRows)
	if beforeKeys == "" {
		t.Fatal("account.getAuthorizations returned no sessions")
	}

	_, err := c.Invoke(ctx, &mtproto.TLAccountSetAuthorizationTTL{
		Constructor:          mtproto.CRC32_account_setAuthorizationTTL,
		AuthorizationTtlDays: 6,
	})
	rpc, ok := asRPCError(err)
	if err == nil || !ok || !strings.Contains(rpc.Message, "TTL_DAYS_INVALID") {
		t.Fatalf("6 days: %v", err)
	}
	if days, rows := readAuthorizations(t, c); days == 6 || authorizationKeys(rows) != beforeKeys {
		t.Fatalf("rejected 6 days stored %d or changed sessions %q -> %q", days, beforeKeys, authorizationKeys(rows))
	}

	stored := invokeOK(t, c, &mtproto.TLAccountSetAuthorizationTTL{
		Constructor:          mtproto.CRC32_account_setAuthorizationTTL,
		AuthorizationTtlDays: 7,
	})
	if _, yes := stored.(*mtproto.TLBoolTrue); !yes {
		t.Fatalf("set 7 days: %T", stored)
	}

	days, rows := readAuthorizations(t, c)
	if days != 7 {
		t.Fatalf("account.getAuthorizations authorization_ttl_days = %d", days)
	}
	if got := authorizationKeys(rows); got != beforeKeys {
		t.Fatalf("setAuthorizationTTL changed sessions %q -> %q", beforeKeys, got)
	}
	if _, _, _, err = c.State(ctx); err != nil {
		t.Fatalf("session logged out after setAuthorizationTTL: %v", err)
	}
	if got := accountTTLDays(invokeOK(t, c, &mtproto.TLAccountGetAccountTTL{
		Constructor: mtproto.CRC32_account_getAccountTTL,
	})); got != accountBefore {
		t.Fatalf("account.getAccountTTL changed from %d to %d", accountBefore, got)
	}
}

func readAuthorizations(t *testing.T, c *Client) (int32, []*mtproto.Authorization) {
	t.Helper()
	res := invokeOK(t, c, &mtproto.TLAccountGetAuthorizations{
		Constructor: mtproto.CRC32_account_getAuthorizations,
	})
	list, ok := res.(*mtproto.TLAccountAuthorizations)
	if !ok || list == nil || list.GetData2() == nil {
		t.Fatalf("account.getAuthorizations returned %T", res)
	}
	return list.GetAuthorizationTtlDays(), list.GetAuthorizations()
}

func authorizationKeys(rows []*mtproto.Authorization) string {
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		current := "0"
		if row.GetCurrent() {
			current = "1"
		}
		parts = append(parts, strings.Join([]string{
			strconv.FormatInt(row.GetHash(), 10),
			row.GetDeviceModel(),
			current,
		}, "/"))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
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
