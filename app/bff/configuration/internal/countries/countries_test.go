package countries

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
)

func TestCountriesListIncludesChinaAndRespectsHash(t *testing.T) {
	list := List("en", 0)
	if list.GetPredicateName() != mtproto.Predicate_help_countriesList {
		t.Fatalf("predicate %s", list.GetPredicateName())
	}
	if len(list.GetCountries()) < 200 {
		t.Fatalf("country count %d", len(list.GetCountries()))
	}
	cn := findCountry(list, "CN")
	if cn == nil {
		t.Fatal("CN missing")
	}
	if cn.GetDefaultName() != "China" {
		t.Fatalf("default name %q", cn.GetDefaultName())
	}
	if cn.GetName().GetValue() != "China" {
		t.Fatalf("english name %q", cn.GetName().GetValue())
	}
	if len(cn.GetCountryCodes()) != 1 || cn.GetCountryCodes()[0].GetCountryCode() != "86" {
		t.Fatalf("codes %+v", cn.GetCountryCodes())
	}
	pattern := cn.GetCountryCodes()[0].GetPatterns()
	if len(pattern) != 1 || !containsX(pattern[0]) {
		t.Fatalf("pattern %v", pattern)
	}

	again := List("en", list.GetHash())
	if again.GetPredicateName() != mtproto.Predicate_help_countriesListNotModified {
		t.Fatalf("cached hash predicate %s", again.GetPredicateName())
	}
	if List("en", 1).GetPredicateName() != mtproto.Predicate_help_countriesList {
		t.Fatal("wrong hash must return the list")
	}
}

func TestCountriesListLocalizesChinese(t *testing.T) {
	hans := List("zh-hans", 0)
	hant := List("zh-hant", 0)
	cnHans := findCountry(hans, "CN")
	cnHant := findCountry(hant, "CN")
	if cnHans == nil || cnHans.GetName().GetValue() != "中国" || cnHans.GetDefaultName() != "China" {
		t.Fatalf("zh-hans %+v", cnHans)
	}
	if cnHant == nil || cnHant.GetName().GetValue() != "中國" {
		t.Fatalf("zh-hant name %q", cnHant.GetName().GetValue())
	}
	twHans := findCountry(hans, "TW")
	twHant := findCountry(hant, "TW")
	if twHans == nil || twHant == nil {
		t.Fatal("TW missing")
	}
	if twHans.GetName().GetValue() == twHant.GetName().GetValue() {
		t.Fatalf("zh-hans %q and zh-hant %q should differ for TW", twHans.GetName().GetValue(), twHant.GetName().GetValue())
	}
	if hans.GetHash() == hant.GetHash() {
		t.Fatal("localized lists must not share a hash")
	}
	if List("zh_CN", hans.GetHash()).GetPredicateName() != mtproto.Predicate_help_countriesListNotModified {
		t.Fatal("zh_CN should use the zh-hans list")
	}
}

func TestSharedCallingCodePrefixes(t *testing.T) {
	list := List("en", 0)
	us := findCountry(list, "US")
	ca := findCountry(list, "CA")
	if us == nil || ca == nil {
		t.Fatal("US/CA missing")
	}
	if us.GetCountryCodes()[0].GetCountryCode() != "1" || ca.GetCountryCodes()[0].GetCountryCode() != "1" {
		t.Fatal("NANP calling code")
	}
	if len(us.GetCountryCodes()[0].GetPrefixes()) != 0 {
		t.Fatalf("US is the default +1 region and should not carry prefixes: %v", us.GetCountryCodes()[0].GetPrefixes())
	}
	if len(ca.GetCountryCodes()[0].GetPrefixes()) == 0 {
		t.Fatal("Canada should carry +1 prefixes")
	}
}

func TestDefaultCountry(t *testing.T) {
	t.Setenv("TELLOMI_DEFAULT_COUNTRY", "")
	if DefaultCountry() != "CN" {
		t.Fatalf("default %s", DefaultCountry())
	}
	t.Setenv("TELLOMI_DEFAULT_COUNTRY", "us")
	if DefaultCountry() != "US" {
		t.Fatalf("override %s", DefaultCountry())
	}
	t.Setenv("TELLOMI_DEFAULT_COUNTRY", "china")
	if DefaultCountry() != "CN" {
		t.Fatalf("invalid override %s", DefaultCountry())
	}
}

func TestCountriesListIncludesHiddenTestCallingCode(t *testing.T) {
	list := List("en", 0)
	test := findCountry(list, testCountryISO2)
	if test == nil {
		t.Fatal("test country missing")
	}
	if !test.GetHidden() {
		t.Fatal("test country must be hidden so pickers that honor the flag omit it")
	}
	if test.GetDefaultName() != testCountryName || test.GetName().GetValue() != testCountryName {
		t.Fatalf("name default %q localized %q", test.GetDefaultName(), test.GetName().GetValue())
	}
	codes := test.GetCountryCodes()
	if len(codes) != 1 {
		t.Fatalf("codes %d", len(codes))
	}
	cc := codes[0]
	if cc.GetCountryCode() != testCountryCode {
		t.Fatalf("calling code %q", cc.GetCountryCode())
	}
	if len(cc.GetPrefixes()) != 1 || cc.GetPrefixes()[0] != testCountryPrefix {
		t.Fatalf("prefixes %v", cc.GetPrefixes())
	}
	if len(cc.GetPatterns()) != 1 || cc.GetPatterns()[0] != testCountryPattern {
		t.Fatalf("patterns %v", cc.GetPatterns())
	}
	if patternSlots(testCountryPattern) != 7 {
		t.Fatalf("national pattern %q must be 7 slots (66XYYYY)", testCountryPattern)
	}

	hidden := 0
	for _, c := range list.GetCountries() {
		if c.GetHidden() {
			hidden++
			if c.GetIso2() != testCountryISO2 {
				t.Fatalf("unexpected hidden country %s", c.GetIso2())
			}
		}
		for _, code := range c.GetCountryCodes() {
			if c.GetIso2() != testCountryISO2 && (code.GetCountryCode() == testCountryCode || strings.HasPrefix(code.GetCountryCode(), testCountryCode) || strings.HasPrefix(testCountryCode, code.GetCountryCode())) {
				t.Fatalf("real country %s uses calling code %q", c.GetIso2(), code.GetCountryCode())
			}
		}
	}
	if hidden != 1 {
		t.Fatalf("hidden countries %d", hidden)
	}

	zh := findCountry(List("zh-hans", 0), testCountryISO2)
	if zh == nil || zh.GetName().GetValue() != testCountryName || !zh.GetHidden() {
		t.Fatal("localized list must keep the same hidden test country")
	}

	wire := encodeDecodeCountries(t, list)
	again := findCountry(wire, testCountryISO2)
	if again == nil || !again.GetHidden() {
		t.Fatal("hidden flag did not survive TL encode")
	}
	if len(again.GetCountryCodes()) != 1 || again.GetCountryCodes()[0].GetCountryCode() != testCountryCode {
		t.Fatal("calling code did not survive TL encode")
	}
	if got := again.GetCountryCodes()[0].GetPrefixes(); len(got) != 1 || got[0] != testCountryPrefix {
		t.Fatalf("prefixes after decode %v", got)
	}
	if got := again.GetCountryCodes()[0].GetPatterns(); len(got) != 1 || got[0] != testCountryPattern {
		t.Fatalf("patterns after decode %v", got)
	}
	if wire.GetHash() != list.GetHash() {
		t.Fatalf("hash %d vs decoded %d", list.GetHash(), wire.GetHash())
	}
}

func TestTestCallingCodeDoesNotBypassSMS(t *testing.T) {
	phonenumber.SetTestPhoneRanges(nil)
	t.Cleanup(func() { phonenumber.SetTestPhoneRanges(nil) })

	// +999 6612001 is 99966XYYYY with X = 1, so the existing dev code is 11111.
	code, ok := phonenumber.TestPhoneCode("+999 6612001")
	if !ok || code != "11111" {
		t.Fatalf("test code = %q ok=%v", code, ok)
	}
	if _, _, err := phonenumber.CheckPhoneNumberInvalid("+999 6612001"); err != nil {
		t.Fatal(err)
	}
	for _, phone := range []string{"+9991234567", "9996712345", "+86 138 0013 8000"} {
		if _, ok := phonenumber.TestPhoneCode(phone); ok {
			t.Fatalf("%s must not be a test phone", phone)
		}
	}
	if _, _, err := phonenumber.CheckPhoneNumberInvalid("+9991234567"); err == nil {
		t.Fatal("non-test +999 was accepted")
	}
	region, digits, err := phonenumber.CheckPhoneNumberInvalid("+86 138 0013 8000")
	if err != nil || region != "CN" || digits != "8613800138000" {
		t.Fatalf("real number region %q digits %q err %v", region, digits, err)
	}
}

func encodeDecodeCountries(t *testing.T, list *mtproto.Help_CountriesList) *mtproto.Help_CountriesList {
	t.Helper()
	buf := mtproto.NewEncodeBuf(1 << 20)
	if err := list.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	decoded := &mtproto.Help_CountriesList{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	if decoded.GetPredicateName() != mtproto.Predicate_help_countriesList {
		t.Fatalf("decoded predicate %s", decoded.GetPredicateName())
	}
	return decoded
}

func patternSlots(pattern string) int {
	n := 0
	for _, r := range pattern {
		if r != ' ' {
			n++
		}
	}
	return n
}

func findCountry(list *mtproto.Help_CountriesList, iso string) *mtproto.Help_Country {
	for _, c := range list.GetCountries() {
		if c.GetIso2() == iso {
			return c
		}
	}
	return nil
}

func containsX(s string) bool {
	for _, r := range s {
		if r == 'X' {
			return true
		}
	}
	return false
}
