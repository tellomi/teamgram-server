package countries

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
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
