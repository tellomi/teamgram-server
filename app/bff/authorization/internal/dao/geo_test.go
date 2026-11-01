package dao

import (
	"testing"

	"github.com/teamgram/teamgram-server/pkg/geoip/testdb"
)

func TestGetCountryAndRegionByIpMissingDB(t *testing.T) {
	d := &Dao{}
	region, extra := d.GetCountryAndRegionByIp("8.8.8.8")
	if region != "8.8.8.8" || extra != "" || region == "UNKNOWN" {
		t.Fatalf("missing db = %q %q", region, extra)
	}
	region, extra = d.GetCountryAndRegionByIp("")
	if region != "" || extra != "" {
		t.Fatalf("empty = %q %q", region, extra)
	}
}

func TestGetCountryAndRegionByIpPublic(t *testing.T) {
	d := &Dao{geo: testdb.Open(t)}
	region, extra := d.GetCountryAndRegionByIp("171.221.32.1")
	if region != "中国 四川 成都" || extra != "四川 成都" {
		t.Fatalf("chengdu = %q %q", region, extra)
	}
	region, extra = d.GetCountryAndRegionByIp("8.8.8.8:443")
	if region != "United States" || extra != "" {
		t.Fatalf("abroad = %q %q", region, extra)
	}
	if region == "UNKNOWN" {
		t.Fatal("stub UNKNOWN")
	}
}
