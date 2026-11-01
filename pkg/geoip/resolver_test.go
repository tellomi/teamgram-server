package geoip_test

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/geoip"
	"github.com/teamgram/teamgram-server/pkg/geoip/testdb"
)

func TestOpenMissingDatabaseDoesNotFailLookup(t *testing.T) {
	dir := t.TempDir()
	r := geoip.Open(geoip.Options{
		V4Path:   filepath.Join(dir, "missing-v4.xdb"),
		V6Path:   filepath.Join(dir, "missing-v6.xdb"),
		MMDBPath: filepath.Join(dir, "missing.mmdb"),
	})
	country, region := r.Lookup("8.8.8.8")
	if country != "" || region != "" {
		t.Fatalf("missing xdb = %q %q", country, region)
	}

	r = geoip.Open(geoip.Options{
		MMDBPath: filepath.Join(dir, "missing.mmdb"),
		V4Path:   filepath.Join(dir, "ignored.xdb"),
		V6Path:   filepath.Join(dir, "ignored-v6.xdb"),
	})
	country, region = r.Lookup("1.1.1.1")
	if country != "" || region != "" {
		t.Fatalf("missing mmdb = %q %q", country, region)
	}

	if country, region := (*geoip.Resolver)(nil).Lookup("8.8.8.8"); country != "" || region != "" {
		t.Fatalf("nil resolver = %q %q", country, region)
	}
}

func TestSampleIPs(t *testing.T) {
	r := testdb.Open(t)

	cases := []struct {
		ip      string
		country string
		region  string
	}{
		{ip: "171.221.32.1", country: "中国", region: "四川 成都"},
		{ip: "171.221.32.1:443", country: "中国", region: "四川 成都"},
		{ip: "180.76.76.76", country: "中国", region: "北京"},
		{ip: "114.114.114.114", country: "中国", region: "江苏 南京"},
		{ip: "203.80.0.1", country: "中国", region: "香港"},
		{ip: "8.8.8.8", country: "United States", region: ""},
		{ip: "1.1.1.1", country: "Australia", region: ""},
		{ip: "240e:3b7:3272:d8d0:db09:c067:8d59:539e", country: "中国", region: "广东 深圳"},
		{ip: "2001:4860:4860::8888", country: "United States", region: ""},
	}
	for _, tc := range cases {
		country, region := r.Lookup(tc.ip)
		if country != tc.country || region != tc.region {
			t.Fatalf("%s = %q %q, want %q %q", tc.ip, country, region, tc.country, tc.region)
		}
		if country == "UNKNOWN" || region == "UNKNOWN" {
			t.Fatalf("stub for %s", tc.ip)
		}
		if geoip.LocationLine(country, region) == "" {
			t.Fatalf("empty place for %s", tc.ip)
		}
	}

	for _, ip := range []string{"10.0.0.1", "127.0.0.1", "192.168.1.1", "::1", "", "not-an-ip"} {
		country, region := r.Lookup(ip)
		if country != "" || region != "" {
			t.Fatalf("unrouted %s = %q %q", ip, country, region)
		}
	}
}

func TestMissingCityDBUsesIP2Region(t *testing.T) {
	v4, v6 := testdb.Files(t)
	r := geoip.Open(geoip.Options{
		V4Path:   v4,
		V6Path:   v6,
		MMDBPath: filepath.Join(t.TempDir(), "missing.mmdb"),
	})
	country, region := r.Lookup("171.221.32.1")
	if country != "中国" || region != "四川 成都" {
		t.Fatalf("chengdu without mmdb = %q %q", country, region)
	}
	country, region = r.Lookup("8.8.8.8")
	if country != "United States" || region != "" {
		t.Fatalf("abroad without mmdb = %q %q", country, region)
	}
	country, region = r.Lookup("203.80.0.1")
	if country != "中国" || region != "香港" {
		t.Fatalf("hong kong without mmdb = %q %q", country, region)
	}
}

func TestLookupConcurrent(t *testing.T) {
	r := testdb.Open(t)
	var wg sync.WaitGroup
	errCh := make(chan string, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			country, region := r.Lookup("171.221.32.1")
			if country != "中国" || region != "四川 成都" {
				errCh <- country + "/" + region
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Fatalf("concurrent lookup = %s", msg)
	}
}

func TestLoadIP2RegionIgnoresMissingMaxMind(t *testing.T) {
	v4, v6 := testdb.Files(t)
	t.Setenv("IP2REGION_V4_XDB", v4)
	t.Setenv("IP2REGION_V6_XDB", v6)
	t.Setenv("MMDB", filepath.Join(t.TempDir(), "missing.mmdb"))
	t.Setenv("GEOIP_MMDB", "")
	t.Setenv("TELLOMI_GEOIP_MMDB", "")
	r := geoip.Load("./GeoLite2-City.mmdb")
	country, region := r.Lookup("8.8.8.8")
	if country != "United States" || region != "" {
		t.Fatalf("load ip2region = %q %q", country, region)
	}
	country, region = r.Lookup("171.221.32.1")
	if country != "中国" || region != "四川 成都" {
		t.Fatalf("load chengdu = %q %q", country, region)
	}
}

func TestLoadMaxMindEnvMissDoesNotPanic(t *testing.T) {
	t.Setenv("MMDB", filepath.Join(t.TempDir(), "no.mmdb"))
	t.Setenv("GEOIP_MMDB", "")
	t.Setenv("TELLOMI_GEOIP_MMDB", "")
	t.Setenv("IP2REGION_V4_XDB", filepath.Join(t.TempDir(), "no-v4.xdb"))
	t.Setenv("IP2REGION_V6_XDB", filepath.Join(t.TempDir(), "no-v6.xdb"))
	r := geoip.Load("./GeoLite2-City.mmdb")
	country, region := r.Lookup("8.8.8.8")
	if country != "" || region != "" {
		t.Fatalf("load miss = %q %q", country, region)
	}
}
