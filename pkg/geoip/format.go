package geoip

import (
	"net"
	"strings"
)

// ParseClientIP accepts a host, host:port, [ipv6]:port, or the "ip|stamp"
// close marker stored on some sessions and returns a parsed address.
func ParseClientIP(raw string) net.IP {
	ip := strings.TrimSpace(raw)
	if ip == "" {
		return nil
	}
	if i := strings.IndexByte(ip, '|'); i >= 0 {
		ip = strings.TrimSpace(ip[:i])
	}
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return net.ParseIP(strings.Trim(ip, "[]"))
}

// LocationLine is the single place string used by the sign-in service
// notice. Domestic lookups read like "中国 四川 成都". A foreign lookup is
// the country name alone.
func LocationLine(country, region string) string {
	country = strings.TrimSpace(country)
	region = strings.TrimSpace(region)
	if country == "" {
		return region
	}
	if region == "" {
		return country
	}
	return country + " " + region
}

// FormatIP2Region maps one ip2region xdb line onto authorization.country
// and authorization.region.
//
// The line is 国家|省份|城市|ISP|ISO. The ISP is not a place, and a token
// of "0", "Reserved", or "UNKNOWN" is empty. Telegram clients join the two
// fields with a comma, so a Chengdu address is stored as country "中国" and
// region "四川 成都" and shown as "中国, 四川 成都". A foreign address fills
// country with the database's country name and leaves region empty.
func FormatIP2Region(raw string) (country, region string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	parts := strings.Split(raw, "|")
	field := func(i int) string {
		if i < 0 || i >= len(parts) {
			return ""
		}
		return parts[i]
	}
	country = cleanToken(field(0))
	iso := strings.TrimSpace(field(4))
	if country == "" && !strings.EqualFold(iso, "CN") {
		return "", ""
	}
	if country == "" {
		country = "中国"
	}
	if !domesticChina(country, iso) {
		return country, ""
	}
	province := cleanAdmin(field(1))
	city := cleanAdmin(field(2))
	switch {
	case province == "":
		region = city
	case city == "" || city == province:
		region = province
	default:
		region = province + " " + city
	}
	return country, region
}

// MainlandChina reports whether an ip2region line is mainland China.
// Hong Kong, Macao, and Taiwan are not mainland; those addresses use the
// city mmdb. An empty or reserved line is not mainland.
func MainlandChina(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	parts := strings.Split(raw, "|")
	field := func(i int) string {
		if i < 0 || i >= len(parts) {
			return ""
		}
		return parts[i]
	}
	country := cleanToken(field(0))
	iso := strings.TrimSpace(field(4))
	if country != "中国" && !strings.EqualFold(iso, "CN") {
		return false
	}
	switch cleanAdmin(field(1)) {
	case "香港", "澳门", "澳門", "台湾", "台灣":
		return false
	}
	return true
}

// ChoosePlace picks authorization.country and authorization.region.
// A mainland China ip2region hit wins, because its province and city are
// more accurate. Any other address uses the city mmdb (DB-IP Lite or
// GeoLite2). When that database has nothing, the ip2region country is used.
// Both inputs may be empty.
func ChoosePlace(ip2Raw, mmdbCountry, mmdbRegion string) (string, string) {
	if MainlandChina(ip2Raw) {
		country, region := FormatIP2Region(ip2Raw)
		if country != "" {
			return country, region
		}
	}
	mmdbCountry = strings.TrimSpace(mmdbCountry)
	mmdbRegion = strings.TrimSpace(mmdbRegion)
	if strings.EqualFold(mmdbCountry, "UNKNOWN") {
		mmdbCountry = ""
	}
	if strings.EqualFold(mmdbRegion, "UNKNOWN") {
		mmdbRegion = ""
	}
	if mmdbCountry != "" {
		return mmdbCountry, mmdbRegion
	}
	return FormatIP2Region(ip2Raw)
}

// FormatMaxMind maps a city mmdb record (DB-IP Lite or GeoLite2-City) onto
// authorization.country and authorization.region. Empty names stay empty.
// The historical stub "UNKNOWN" is never returned; clients print that
// string on the device line.
func FormatMaxMind(countryName, countryISO, regionName, cityName string) (string, string) {
	country := strings.TrimSpace(countryName)
	if country == "" {
		country = strings.TrimSpace(countryISO)
	}
	region := strings.TrimSpace(regionName)
	if region == "" {
		region = strings.TrimSpace(cityName)
	}
	if region == "" {
		region = strings.TrimSpace(countryISO)
	}
	if country == "" {
		country = strings.TrimSpace(cityName)
	}
	if strings.EqualFold(country, "UNKNOWN") {
		country = ""
	}
	if strings.EqualFold(region, "UNKNOWN") {
		region = ""
	}
	return country, region
}

func domesticChina(country, iso string) bool {
	if country == "中国" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(iso), "CN")
}

func cleanToken(s string) string {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "", "0", "unknown", "reserved", "内网ip":
		return ""
	default:
		return s
	}
}

func cleanAdmin(s string) string {
	s = cleanToken(s)
	if s == "" {
		return ""
	}
	suffixes := []string{
		"维吾尔自治区",
		"回族自治区",
		"壮族自治区",
		"特别行政区",
		"自治区",
		"省",
		"市",
	}
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) && len([]rune(s)) > len([]rune(suffix)) {
			return strings.TrimSpace(strings.TrimSuffix(s, suffix))
		}
	}
	return s
}
