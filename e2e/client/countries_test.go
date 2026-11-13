package e2e

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// TestHelpGetCountriesListIncludesTestCallingCode is the release-client
// contract for dev numbers 99966XYYYY. Android LoginActivity replaces
// assets/countries.txt with this list and will not call auth.sendCode
// unless country code 999 is present. The row stays hidden so clients
// that honor the flag leave it out of the picker.
func TestHelpGetCountriesListIncludesTestCallingCode(t *testing.T) {
	a, _, _, _ := sharedPair(t)

	en := fetchCountries(t, a, "en", 0)
	test := findTestCountry(t, en)
	if !test.GetHidden() {
		t.Fatal("test country must be hidden")
	}
	if test.GetIso2() != "XT" || test.GetDefaultName() != "Test" || test.GetName().GetValue() != "Test" {
		t.Fatalf("test country %+v", test)
	}
	codes := test.GetCountryCodes()
	if len(codes) != 1 || codes[0].GetCountryCode() != "999" {
		t.Fatalf("calling codes %+v", codes)
	}
	if got := codes[0].GetPrefixes(); len(got) != 1 || got[0] != "66" {
		t.Fatalf("prefixes %v", got)
	}
	if got := codes[0].GetPatterns(); len(got) != 1 || got[0] != "66X XXXX" {
		t.Fatalf("patterns %v", got)
	}
	if findCountryISO(en, "CN") == nil {
		t.Fatal("CN missing")
	}
	hidden := 0
	for _, c := range en.GetCountries() {
		if c.GetHidden() {
			hidden++
		}
	}
	if hidden != 1 {
		t.Fatalf("hidden countries %d", hidden)
	}

	cached := invokeOK(t, a, &mtproto.TLHelpGetCountriesList{
		Constructor: mtproto.CRC32_help_getCountriesList,
		LangCode:    "en",
		Hash:        en.GetHash(),
	})
	if _, ok := cached.(*mtproto.TLHelpCountriesListNotModified); !ok {
		if list, ok := cached.(*mtproto.Help_CountriesList); !ok || list.GetPredicateName() != mtproto.Predicate_help_countriesListNotModified {
			t.Fatalf("matching hash returned %T", cached)
		}
	}

	zh := fetchCountries(t, a, "zh-hans", 0)
	zhTest := findTestCountry(t, zh)
	if zhTest.GetName().GetValue() != "Test" || !zhTest.GetHidden() {
		t.Fatal("zh-hans list dropped the hidden test country")
	}
	if zh.GetHash() == en.GetHash() {
		t.Fatal("localized lists must not share a hash")
	}
}

func fetchCountries(t *testing.T, c *Client, lang string, hash int32) *mtproto.Help_CountriesList {
	t.Helper()
	obj := invokeOK(t, c, &mtproto.TLHelpGetCountriesList{
		Constructor: mtproto.CRC32_help_getCountriesList,
		LangCode:    lang,
		Hash:        hash,
	})
	switch v := obj.(type) {
	case *mtproto.TLHelpCountriesList:
		return v.To_Help_CountriesList()
	case *mtproto.Help_CountriesList:
		if v.GetPredicateName() != mtproto.Predicate_help_countriesList {
			t.Fatalf("predicate %s", v.GetPredicateName())
		}
		return v
	default:
		t.Fatalf("help.getCountriesList returned %T", obj)
	}
	return nil
}

func findTestCountry(t *testing.T, list *mtproto.Help_CountriesList) *mtproto.Help_Country {
	t.Helper()
	c := findCountryISO(list, "XT")
	if c == nil {
		t.Fatal("country 999 / XT missing from help.getCountriesList")
	}
	return c
}

func findCountryISO(list *mtproto.Help_CountriesList, iso string) *mtproto.Help_Country {
	for _, c := range list.GetCountries() {
		if c.GetIso2() == iso {
			return c
		}
	}
	return nil
}
