package dao

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

func TestGeoIPPathPrefersEnv(t *testing.T) {
	t.Setenv("TELLOMI_GEOIP_MMDB", "")
	t.Setenv("MMDB", "/var/lib/tellomi/GeoLite2-City.mmdb")
	t.Setenv("GEOIP_MMDB", "/tmp/other.mmdb")
	if got := geoIPPath(); got != "/var/lib/tellomi/GeoLite2-City.mmdb" {
		t.Fatalf("MMDB = %q", got)
	}
	t.Setenv("MMDB", "")
	if got := geoIPPath(); got != "/tmp/other.mmdb" {
		t.Fatalf("GEOIP_MMDB = %q", got)
	}
	t.Setenv("GEOIP_MMDB", "")
	if got := geoIPPath(); got != mmdb {
		t.Fatalf("flag path = %q, want %q", got, mmdb)
	}
}

func TestKeptAuthorizationHashSurvivesALiveRebind(t *testing.T) {
	if keptAuthorizationHash(11, false, 99) != 11 {
		t.Fatal("live hash replaced")
	}
	if keptAuthorizationHash(11, true, 99) != 99 {
		t.Fatal("revoked row kept the old hash")
	}
	if keptAuthorizationHash(0, false, 99) != 99 {
		t.Fatal("zero hash stayed zero")
	}
	if got := keptAuthorizationHash(0, true, 0); got == 0 {
		t.Fatal("new login stored hash 0")
	}
}

func TestBindDateCreatedRestartsOnlyAClearedRow(t *testing.T) {
	now := int64(1_700_000_000)
	if bindDateCreated(now-48*3600, now) != now-48*3600 {
		t.Fatal("a live session keeps its age")
	}
	if bindDateCreated(0, now) != now {
		t.Fatal("a cleared created time starts at this bind")
	}
	if bindDateCreated(-5, now) != now {
		t.Fatal("a negative stored time starts at this bind")
	}
	if bindDateCreated(0, 0) != 0 {
		t.Fatal("no clock")
	}
}

func TestFreshResetWindowIsCurrentSessionAge(t *testing.T) {
	now := int64(1_700_000_000)
	older := now - 48*3600
	newer := now - 3600

	if !freshResetForbidden(newer, now) {
		t.Fatal("a session logged on 1h ago must be refused")
	}
	if freshResetForbidden(older, now) {
		t.Fatal("a session logged on 48h ago must be allowed")
	}
	if freshResetForbidden(newer, newer+freshResetWindowSeconds) {
		t.Fatal("the same session must be allowed at exactly 24h")
	}
	if !freshResetForbidden(newer, newer+freshResetWindowSeconds-1) {
		t.Fatal("the same session must be refused at 24h-1s")
	}
	if freshResetForbidden(0, now) || freshResetForbidden(-1, now) {
		t.Fatal("a missing created time must not lock the account out")
	}
	if freshResetForbidden(now+30, now) {
		t.Fatal("a created time in the future must not lock the account out")
	}
	if !freshResetForbidden(now, now) {
		t.Fatal("a session created this second is still inside the window")
	}
}

func TestResetDecisionDoesNotDeleteInsideTheWindow(t *testing.T) {
	now := int64(1_700_000_000)
	rows := []authUserRef{
		{RowID: 1, AuthKeyID: 100, Hash: 11},
		{RowID: 2, AuthKeyID: 200, Hash: 22},
	}
	// Device 2 logged on an hour ago. Device 1 has been on for two days.
	// Login order does not matter: only the actor's created time does.
	err := ResetAuthorizationError(200, now-3600, now, 11, true)
	if !errors.Is(err, mtproto.ErrFreshResetAuthorisationForbidden) {
		t.Fatalf("fresh actor: %v", err)
	}
	_, stayed := authUsersToReset(rows, 200, 11)
	if len(stayed) != 1 || stayed[0] != 100 {
		t.Fatalf("selection still names the old key %v; the handler must return before using it", stayed)
	}

	if err = ResetAuthorizationError(100, now-48*3600, now, 22, true); err != nil {
		t.Fatalf("old actor matching hash: %v", err)
	}
	_, keys := authUsersToReset(rows, 100, 22)
	if len(keys) != 1 || keys[0] != 200 {
		t.Fatalf("old actor should revoke the other key, got %v", keys)
	}

	err = ResetAuthorizationError(100, now-48*3600, now, 99, false)
	if !errors.Is(err, mtproto.ErrHashInvalid) {
		t.Fatalf("missing hash: %v", err)
	}
	_, keys = authUsersToReset(rows, 100, 99)
	if len(keys) != 0 {
		t.Fatalf("missing hash selected %v", keys)
	}

	err = ResetAuthorizationError(100, now-48*3600, now, 11, false)
	if !errors.Is(err, mtproto.ErrHashInvalid) {
		t.Fatalf("own hash: %v", err)
	}

	if err = ResetAuthorizationError(0, now, now, 0, true); err != nil {
		t.Fatalf("account deletion must skip the 24h gate: %v", err)
	}
	_, keys = authUsersToReset(rows, 0, 0)
	if len(keys) != 2 {
		t.Fatalf("account deletion keys = %v", keys)
	}
}

func TestUnionAuthKeyIDsKeepsPermAndTemp(t *testing.T) {
	got := UnionAuthKeyIDs([]int64{10, 0, 20, 10, 21, 0})
	want := []int64{10, 20, 21}
	if len(got) != len(want) {
		t.Fatalf("ids = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v", got)
		}
	}
}

func TestAuthUsersToResetDropsOtherKeys(t *testing.T) {
	rows := []authUserRef{
		{RowID: 1, AuthKeyID: 100, Hash: 11},
		{RowID: 2, AuthKeyID: 200, Hash: 22},
		{RowID: 3, AuthKeyID: 300, Hash: 33},
	}

	rowIDs, keyIDs := authUsersToReset(rows, 100, 0)
	if len(keyIDs) != 2 || keyIDs[0] != 200 || keyIDs[1] != 300 {
		t.Fatalf("reset all keys = %v, want 200,300", keyIDs)
	}
	if len(rowIDs) != 2 || rowIDs[0] != 2 || rowIDs[1] != 3 {
		t.Fatalf("reset all row ids = %v, want 2,3", rowIDs)
	}

	rowIDs, keyIDs = authUsersToReset(rows, 100, 22)
	if len(keyIDs) != 1 || keyIDs[0] != 200 || rowIDs[0] != 2 {
		t.Fatalf("reset one = rows %v keys %v", rowIDs, keyIDs)
	}

	_, keyIDs = authUsersToReset(rows, 100, 11)
	if len(keyIDs) != 0 {
		t.Fatalf("current hash must not drop the caller, got %v", keyIDs)
	}

	_, keyIDs = authUsersToReset(rows, 0, 0)
	if len(keyIDs) != 3 {
		t.Fatalf("exclude nobody = %v, want all three", keyIDs)
	}
}

func TestAuthorizationPlaceUsesIPWhenGeoIPMissing(t *testing.T) {
	d := &Dao{}

	ip, country, region := d.authorizationPlace("8.8.8.8")
	if ip != "8.8.8.8" || country != "8.8.8.8" || region != "" {
		t.Fatalf("place = %q %q %q", ip, country, region)
	}
	if country == "UNKNOWN" {
		t.Fatal("country stub UNKNOWN")
	}

	ip, country, region = d.authorizationPlace("8.8.8.8:443")
	if ip != "8.8.8.8" || country != "8.8.8.8" {
		t.Fatalf("host:port place = %q %q", ip, country)
	}

	ip, country, region = d.authorizationPlace("[2001:db8::1]:443")
	if ip != "2001:db8::1" || country != "2001:db8::1" {
		t.Fatalf("ipv6 place = %q %q", ip, country)
	}

	ip, country, region = d.authorizationPlace("")
	if ip != "" || country != "" || region != "" || country == "UNKNOWN" {
		t.Fatalf("empty place = %q %q %q", ip, country, region)
	}
}

func TestFillAuthorizationPlaceKeepsResolvedCountry(t *testing.T) {
	ip, country, region := fillAuthorizationPlace("8.8.8.8", "United States", "California")
	if ip != "8.8.8.8" || country != "United States" || region != "California" {
		t.Fatalf("resolved place = %q %q %q", ip, country, region)
	}
	_, country, _ = fillAuthorizationPlace("8.8.8.8", "", "")
	if country != "8.8.8.8" {
		t.Fatalf("missing geo country = %q", country)
	}
}

func TestToAuthStateRevokedIsLoggedOut(t *testing.T) {
	revoked := &CacheAuthData{
		Client:  authsession.MakeTLClientSession(&authsession.ClientSession{}).To_ClientSession(),
		Revoked: true,
	}
	if revoked.ToAuthState() != mtproto.AuthStateLogout {
		t.Fatalf("revoked state = %d", revoked.ToAuthState())
	}

	active := &CacheAuthData{BindUser: &BindUser{UserId: 7}}
	if active.ToAuthState() != mtproto.AuthStateNormal {
		t.Fatalf("active state = %d", active.ToAuthState())
	}

	fresh := &CacheAuthData{Client: authsession.MakeTLClientSession(&authsession.ClientSession{}).To_ClientSession()}
	if fresh.ToAuthState() != mtproto.AuthStateUnauthorized {
		t.Fatalf("fresh state = %d", fresh.ToAuthState())
	}
}
