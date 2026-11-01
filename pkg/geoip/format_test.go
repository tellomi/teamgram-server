package geoip

import "testing"

func TestFormatIP2RegionDomestic(t *testing.T) {
	country, region := FormatIP2Region("中国|四川省|成都市|电信|CN")
	if country != "中国" || region != "四川 成都" {
		t.Fatalf("chengdu = %q %q", country, region)
	}
	if got := LocationLine(country, region); got != "中国 四川 成都" {
		t.Fatalf("chengdu line = %q", got)
	}
	if stringsContainsISP(region) {
		t.Fatalf("isp leaked into region %q", region)
	}

	country, region = FormatIP2Region("中国|北京市|北京市|百度|CN")
	if country != "中国" || region != "北京" {
		t.Fatalf("beijing = %q %q", country, region)
	}

	country, region = FormatIP2Region("中国|上海市|上海市|电信|CN")
	if country != "中国" || region != "上海" {
		t.Fatalf("shanghai = %q %q", country, region)
	}

	country, region = FormatIP2Region("中国|香港特别行政区|0|0|CN")
	if country != "中国" || region != "香港" {
		t.Fatalf("hong kong = %q %q", country, region)
	}

	country, region = FormatIP2Region("中国|台湾省|0|中华电信|CN")
	if country != "中国" || region != "台湾" {
		t.Fatalf("taiwan = %q %q", country, region)
	}

	country, region = FormatIP2Region("中国|新疆维吾尔自治区|乌鲁木齐市|电信|CN")
	if country != "中国" || region != "新疆 乌鲁木齐" {
		t.Fatalf("xinjiang = %q %q", country, region)
	}

	country, region = FormatIP2Region("中国|江苏省|0|0|CN")
	if country != "中国" || region != "江苏" {
		t.Fatalf("province only = %q %q", country, region)
	}
}

func TestFormatIP2RegionAbroadIsCountryOnly(t *testing.T) {
	country, region := FormatIP2Region("United States|California|0|Google LLC|US")
	if country != "United States" || region != "" {
		t.Fatalf("us = %q %q", country, region)
	}
	if LocationLine(country, region) != "United States" {
		t.Fatalf("us line = %q", LocationLine(country, region))
	}

	country, region = FormatIP2Region("Australia|Queensland|Brisbane|0|AU")
	if country != "Australia" || region != "" {
		t.Fatalf("au = %q %q", country, region)
	}
}

func TestFormatIP2RegionEmptyAndReserved(t *testing.T) {
	for _, raw := range []string{
		"",
		"0|0|0|0|0",
		"Reserved|Reserved|Reserved|0|0",
		"UNKNOWN|UNKNOWN|UNKNOWN|0|0",
		"内网IP|内网IP|0|0|0",
	} {
		country, region := FormatIP2Region(raw)
		if country != "" || region != "" {
			t.Fatalf("%q -> %q %q", raw, country, region)
		}
		if country == "UNKNOWN" || region == "UNKNOWN" {
			t.Fatalf("stub leaked from %q", raw)
		}
	}
}

func TestChoosePlace(t *testing.T) {
	country, region := ChoosePlace("中国|四川省|成都市|电信|CN", "China", "Sichuan")
	if country != "中国" || region != "四川 成都" {
		t.Fatalf("mainland prefers ip2region: %q %q", country, region)
	}
	if !MainlandChina("中国|四川省|成都市|电信|CN") {
		t.Fatal("chengdu should be mainland")
	}

	country, region = ChoosePlace("中国|香港特别行政区|0|0|CN", "Hong Kong", "Hong Kong")
	if country != "Hong Kong" || region != "Hong Kong" {
		t.Fatalf("hong kong uses mmdb: %q %q", country, region)
	}
	if MainlandChina("中国|香港特别行政区|0|0|CN") || MainlandChina("中国|台湾省|0|中华电信|CN") || MainlandChina("中国|澳门特别行政区|0|0|CN") {
		t.Fatal("HK/MO/TW are not mainland")
	}
	country, region = ChoosePlace("中国|香港特别行政区|0|0|CN", "", "")
	if country != "中国" || region != "香港" {
		t.Fatalf("hong kong without mmdb: %q %q", country, region)
	}

	country, region = ChoosePlace("United States|California|0|Google LLC|US", "United States", "California")
	if country != "United States" || region != "California" {
		t.Fatalf("abroad uses mmdb: %q %q", country, region)
	}
	country, region = ChoosePlace("United States|California|0|Google LLC|US", "", "")
	if country != "United States" || region != "" {
		t.Fatalf("abroad without mmdb: %q %q", country, region)
	}

	country, region = ChoosePlace("", "Germany", "Berlin")
	if country != "Germany" || region != "Berlin" {
		t.Fatalf("mmdb only: %q %q", country, region)
	}
	country, region = ChoosePlace("Reserved|Reserved|Reserved|0|0", "UNKNOWN", "UNKNOWN")
	if country != "" || region != "" {
		t.Fatalf("neither database: %q %q", country, region)
	}
}

func TestFormatMaxMind(t *testing.T) {
	country, region := FormatMaxMind("China", "CN", "Shanghai", "Shanghai")
	if country != "China" || region != "Shanghai" {
		t.Fatalf("got %q %q", country, region)
	}

	country, region = FormatMaxMind("", "CN", "", "Beijing")
	if country != "CN" || region != "Beijing" {
		t.Fatalf("fallback got %q %q", country, region)
	}

	country, region = FormatMaxMind("UNKNOWN", "", "UNKNOWN", "")
	if country != "" || region != "" {
		t.Fatalf("stub leaked: %q %q", country, region)
	}
}

func TestParseClientIP(t *testing.T) {
	if got := ParseClientIP("8.8.8.8:443"); got == nil || got.String() != "8.8.8.8" {
		t.Fatalf("host:port = %v", got)
	}
	if got := ParseClientIP("[2001:db8::1]:443"); got == nil || got.String() != "2001:db8::1" {
		t.Fatalf("ipv6 = %v", got)
	}
	if got := ParseClientIP("1.2.3.4|1710000000"); got == nil || got.String() != "1.2.3.4" {
		t.Fatalf("stamp = %v", got)
	}
	if ParseClientIP("") != nil || ParseClientIP("not-an-ip") != nil {
		t.Fatal("expected nil")
	}
}

func stringsContainsISP(region string) bool {
	return region == "电信" || region == "百度"
}
