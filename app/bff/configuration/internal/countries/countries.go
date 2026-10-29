package countries

import (
	"hash/fnv"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/nyaruka/phonenumbers"
	"github.com/teamgram/proto/mtproto"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// countryCode is one calling-code row inside a country, before localization.
type countryCode struct {
	code     string
	prefixes []string
	pattern  string
}

type countryBase struct {
	iso2  string
	codes []countryCode
}

var (
	countriesOnce sync.Once
	countriesBase []countryBase

	renderedMu sync.Mutex
	rendered   = map[string]*mtproto.Help_CountriesList{}
)

// List is help.getCountriesList. hash 0, or any hash that does not match
// this language's list, returns the full list. A matching hash returns
// help.countriesListNotModified so the client keeps its picker.
func List(langCode string, hash int32) *mtproto.Help_CountriesList {
	lang := normalizeLang(langCode)
	list := renderedList(lang)
	if hash != 0 && hash == list.GetHash() {
		return mtproto.MakeTLHelpCountriesListNotModified(&mtproto.Help_CountriesList{}).To_Help_CountriesList()
	}
	return list
}

func renderedList(lang string) *mtproto.Help_CountriesList {
	renderedMu.Lock()
	defer renderedMu.Unlock()
	if list, ok := rendered[lang]; ok {
		return list
	}
	list := buildCountriesList(lang, loadCountries())
	rendered[lang] = list
	return list
}

func loadCountries() []countryBase {
	countriesOnce.Do(func() {
		countriesBase = buildCountryBase()
	})
	return countriesBase
}

func buildCountryBase() []countryBase {
	regions := phonenumbers.GetSupportedRegions()
	byCode := map[int][]string{}
	var isos []string
	for iso := range regions {
		if len(iso) != 2 {
			continue
		}
		code := phonenumbers.GetCountryCodeForRegion(iso)
		if code <= 0 {
			continue
		}
		isos = append(isos, iso)
		byCode[code] = append(byCode[code], iso)
	}
	sort.Strings(isos)

	prefixes := map[string][]string{}
	for code, group := range byCode {
		if len(group) < 2 {
			continue
		}
		for iso, prefs := range prefixesForCallingCode(code, group) {
			prefixes[iso] = prefs
		}
	}

	out := make([]countryBase, 0, len(isos))
	for _, iso := range isos {
		code := phonenumbers.GetCountryCodeForRegion(iso)
		pattern := callingPattern(iso)
		cc := countryCode{
			code:     strconv.Itoa(code),
			prefixes: prefixes[iso],
			pattern:  pattern,
		}
		out = append(out, countryBase{iso2: iso, codes: []countryCode{cc}})
	}
	return out
}

// prefixesForCallingCode finds the shortest national prefix that tells
// regions sharing a calling code apart. The primary region (US for +1,
// RU for +7) keeps an empty prefix list and acts as the default, matching
// Telegram's country picker.
func prefixesForCallingCode(code int, regions []string) map[string][]string {
	primary := phonenumbers.GetRegionCodeForCountryCode(code)
	want := map[string]bool{}
	for _, iso := range regions {
		if iso != primary {
			want[iso] = true
		}
	}
	if len(want) == 0 {
		return nil
	}
	for length := 1; length <= 3; length++ {
		found := map[string]map[string]struct{}{}
		n := 1
		for i := 0; i < length; i++ {
			n *= 10
		}
		start := 0
		if length > 1 {
			start = n / 10
		}
		for i := start; i < n; i++ {
			prefix := strconv.Itoa(i)
			if length > 1 && len(prefix) < length {
				prefix = strings.Repeat("0", length-len(prefix)) + prefix
			}
			if prefix[0] == '0' {
				continue
			}
			iso := regionForPrefix(code, prefix)
			if iso == "" || iso == primary || !want[iso] {
				continue
			}
			if found[iso] == nil {
				found[iso] = map[string]struct{}{}
			}
			found[iso][prefix] = struct{}{}
		}
		ok := true
		for iso := range want {
			if len(found[iso]) == 0 {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		out := make(map[string][]string, len(found))
		for iso, set := range found {
			prefs := make([]string, 0, len(set))
			for p := range set {
				prefs = append(prefs, p)
			}
			sort.Strings(prefs)
			out[iso] = prefs
		}
		return out
	}
	return nil
}

func regionForPrefix(code int, prefix string) string {
	// Several tails: NANP only classifies the 555-0100 range, while +7 and
	// +44 accept ordinary subscriber numbers. The first tail that resolves
	// wins.
	for _, tail := range []string{"5550100", "1234567", "0009998", "2345678"} {
		raw := "+" + strconv.Itoa(code) + prefix + tail
		num, err := phonenumbers.Parse(raw, "")
		if err != nil || num == nil {
			continue
		}
		if region := phonenumbers.GetRegionCodeForNumber(num); region != "" {
			return region
		}
	}
	return ""
}

func callingPattern(iso string) string {
	num := phonenumbers.GetExampleNumberForType(iso, phonenumbers.MOBILE)
	if num == nil {
		num = phonenumbers.GetExampleNumber(iso)
	}
	if num == nil {
		return "XXX XXX XXXX"
	}
	formatted := phonenumbers.Format(num, phonenumbers.INTERNATIONAL)
	code := strconv.Itoa(phonenumbers.GetCountryCodeForRegion(iso))
	formatted = strings.TrimSpace(strings.TrimPrefix(formatted, "+"+code))
	var b strings.Builder
	prevSpace := false
	for _, r := range formatted {
		switch {
		case r >= '0' && r <= '9':
			b.WriteByte('X')
			prevSpace = false
		case r == ' ' || r == '-' || r == '.':
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	pattern := strings.TrimSpace(b.String())
	if pattern == "" {
		return "XXX XXX XXXX"
	}
	return pattern
}

func buildCountriesList(lang string, base []countryBase) *mtproto.Help_CountriesList {
	tag := nameTag(lang)
	namer := display.Regions(tag)
	english := display.Regions(language.English)

	countries := make([]*mtproto.Help_Country, 0, len(base))
	for _, item := range base {
		region, err := language.ParseRegion(item.iso2)
		defaultName := item.iso2
		localName := item.iso2
		if err == nil {
			if name := english.Name(region); name != "" {
				defaultName = name
			}
			if name := namer.Name(region); name != "" {
				localName = name
			} else {
				localName = defaultName
			}
		}
		codes := make([]*mtproto.Help_CountryCode, 0, len(item.codes))
		for _, cc := range item.codes {
			row := &mtproto.Help_CountryCode{
				CountryCode: cc.code,
				Patterns:    []string{cc.pattern},
			}
			if len(cc.prefixes) > 0 {
				row.Prefixes = append([]string(nil), cc.prefixes...)
			}
			codes = append(codes, mtproto.MakeTLHelpCountryCode(row).To_Help_CountryCode())
		}
		countries = append(countries, mtproto.MakeTLHelpCountry(&mtproto.Help_Country{
			Hidden:       false,
			Iso2:         item.iso2,
			DefaultName:  defaultName,
			Name:         mtproto.MakeFlagsString(localName),
			CountryCodes: codes,
		}).To_Help_Country())
	}
	sort.Slice(countries, func(i, j int) bool {
		ni := countries[i].GetName().GetValue()
		nj := countries[j].GetName().GetValue()
		if ni == nj {
			return countries[i].GetIso2() < countries[j].GetIso2()
		}
		return ni < nj
	})
	return mtproto.MakeTLHelpCountriesList(&mtproto.Help_CountriesList{
		Countries: countries,
		Hash:      countriesHash(lang, countries),
	}).To_Help_CountriesList()
}

func countriesHash(lang string, countries []*mtproto.Help_Country) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(lang))
	for _, c := range countries {
		_, _ = h.Write([]byte(c.GetIso2()))
		_, _ = h.Write([]byte(c.GetDefaultName()))
		_, _ = h.Write([]byte(c.GetName().GetValue()))
		for _, cc := range c.GetCountryCodes() {
			_, _ = h.Write([]byte(cc.GetCountryCode()))
			for _, p := range cc.GetPrefixes() {
				_, _ = h.Write([]byte(p))
			}
			for _, p := range cc.GetPatterns() {
				_, _ = h.Write([]byte(p))
			}
		}
	}
	sum := int32(h.Sum32() & 0x7fffffff)
	if sum == 0 {
		sum = 1
	}
	return sum
}

func normalizeLang(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	c = strings.ReplaceAll(c, "_", "-")
	switch {
	case c == "zh" || c == "zh-cn" || c == "zh-sg" || strings.HasPrefix(c, "zh-hans"):
		return "zh-hans"
	case c == "zh-tw" || c == "zh-hk" || c == "zh-mo" || strings.HasPrefix(c, "zh-hant"):
		return "zh-hant"
	default:
		return "en"
	}
}

func nameTag(lang string) language.Tag {
	switch normalizeLang(lang) {
	case "zh-hans":
		return language.SimplifiedChinese
	case "zh-hant":
		return language.TraditionalChinese
	default:
		return language.English
	}
}

// DefaultCountry is the country help.getNearestDc reports.
// TELLOMI_DEFAULT_COUNTRY overrides it; anything that is not two letters
// falls back to CN.
func DefaultCountry() string {
	c := strings.ToUpper(strings.TrimSpace(os.Getenv("TELLOMI_DEFAULT_COUNTRY")))
	if len(c) == 2 && c[0] >= 'A' && c[0] <= 'Z' && c[1] >= 'A' && c[1] <= 'Z' {
		return c
	}
	return "CN"
}
