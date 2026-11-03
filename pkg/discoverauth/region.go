package discoverauth

import (
	"strings"
	"unicode"

	"github.com/nyaruka/phonenumbers"
)

// ContentRegion is the credential rgn. A mainland account wins over an
// overseas address: country code CN, a +86 phone, or a mainland client
// address. Hong Kong, Macao, and Taiwan are not mainland. An email-only
// account has an empty country code and phone, so only the address applies.
// One MTProto endpoint serves both regions, so there is no entry-host rule.
func ContentRegion(countryCode, phone string, mainlandIP bool) string {
	if strings.EqualFold(strings.TrimSpace(countryCode), "CN") || PhoneCountry86(phone) || mainlandIP {
		return RegionCN
	}
	return RegionGlobal
}

// PhoneCountry86 reports a stored phone whose country calling code is 86.
// Teamgram keeps the digits with the country code and no leading plus
// ("8613800138000").
func PhoneCountry86(phone string) bool {
	digits := digitsOnly(phone)
	if len(digits) < 3 {
		return false
	}
	num, err := phonenumbers.Parse("+"+digits, "")
	if err != nil || num.GetCountryCode() != 86 {
		return false
	}
	return true
}

// MainlandPlace reports the country and region pair from pkg/geoip.Lookup.
// Country 中国 with a Hong Kong, Macao, or Taiwan region is not mainland.
// An empty lookup is not mainland.
func MainlandPlace(country, region string) bool {
	if country != "中国" {
		return false
	}
	region = strings.TrimSpace(region)
	for _, prefix := range []string{"香港", "澳门", "澳門", "台湾", "台灣"} {
		if strings.HasPrefix(region, prefix) {
			return false
		}
	}
	return true
}

func digitsOnly(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
