package dao

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/geoip/testdb"
)

// TestGetAuthorizationsPublicIPCountry walks the place fields that
// GetAuthorization and GetAuthorizations copy onto account.Authorizations.
// The pinned ip2region xdb must yield a country for a public address.
func TestGetAuthorizationsPublicIPCountry(t *testing.T) {
	d := &Dao{geo: testdb.Open(t)}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "171.221.32.1", "171.221.32.1:443"} {
		ip, country, region := d.authorizationPlace(raw)
		auth := mtproto.MakeTLAuthorization(&mtproto.Authorization{
			Current:     false,
			OfficialApp: true,
			Ip:          ip,
			Country:     country,
			Region:      region,
		}).To_Authorization()
		if strings.TrimSpace(auth.GetCountry()) == "" {
			t.Fatalf("getAuthorizations country for %s is empty (region %q)", raw, auth.GetRegion())
		}
		if auth.GetCountry() == "UNKNOWN" || auth.GetCountry() == ip {
			t.Fatalf("getAuthorizations country for %s = %q, want a resolved place", raw, auth.GetCountry())
		}
		if strings.Contains(auth.GetCountry(), "|") || strings.Contains(auth.GetRegion(), "|") {
			t.Fatalf("raw xdb line leaked: %q %q", auth.GetCountry(), auth.GetRegion())
		}
	}

	ip, country, region := d.authorizationPlace("171.221.32.1")
	if ip != "171.221.32.1" || country != "中国" || region != "四川 成都" {
		t.Fatalf("chengdu place = %q %q %q", ip, country, region)
	}

	_, country, region = d.authorizationPlace("8.8.8.8")
	if country != "United States" || region != "" {
		t.Fatalf("abroad place = %q %q", country, region)
	}

	ip, country, region = d.authorizationPlace("10.0.0.1")
	if ip != "10.0.0.1" || country != "10.0.0.1" || region != "" || country == "UNKNOWN" || country == "Reserved" {
		t.Fatalf("private place = %q %q %q", ip, country, region)
	}
}
