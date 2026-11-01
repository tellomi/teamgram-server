package book

import (
	"hash/fnv"
)

// countriesLangs is the help.passportConfig countries_langs document.
// Android reads it as a flat JSON object of ISO country code to language
// (SharedConfig.getCountryLangs). CN uses zh for the mainland default.
// https://core.telegram.org/method/help.getPassportConfig
const countriesLangs = `{"AE":"ar","AU":"en","BR":"pt","CA":"en","CN":"zh","DE":"de","EG":"ar","ES":"es","FR":"fr","GB":"en","HK":"zh-hant","ID":"id","IN":"en","IT":"it","JP":"ja","KR":"ko","MO":"zh-hant","MX":"es","MY":"ms","NL":"nl","PH":"en","RU":"ru","SA":"ar","SG":"en","TH":"th","TR":"tr","TW":"zh-hant","UA":"uk","US":"en","VN":"vi"}`

var configHash int32

func init() {
	h := fnv.New32a()
	_, _ = h.Write([]byte(countriesLangs))
	sum := int32(h.Sum32() & 0x7fffffff)
	if sum == 0 {
		sum = 1
	}
	configHash = sum
}

// CountriesLangs is the DataJSON payload of help.passportConfig.
func CountriesLangs() string { return countriesLangs }

// ConfigHash is the hash clients send back on help.getPassportConfig.
func ConfigHash() int32 { return configHash }
