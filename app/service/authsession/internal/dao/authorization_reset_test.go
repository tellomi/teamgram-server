package dao

import (
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
