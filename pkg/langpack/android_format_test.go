package langpack

import (
	"regexp"
	"sort"
	"strconv"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// printfSpec is a Java Formatter conversion Android's String.format accepts,
// plus %@ which is the iOS token and crashes String.format.
// %% is an escaped percent and is not an argument. A literal "50%" is not a
// conversion: the conversion character has to follow the percent or a width.
var printfSpec = regexp.MustCompile(`%%|%(\d+)\$((?:0?\d+)?)([sdifouxXeEfFgGaAcbBhH@])|%((?:0?\d+)?)([sdifouxXeEfFgGaAcbBhH@])`)

type printfArg struct {
	index int
	kind  string
}

func printfKind(conv string) string {
	switch conv {
	case "d", "D", "i", "o", "u", "x", "X":
		return "int"
	case "f", "F", "e", "E", "g", "G", "a", "A":
		return "float"
	case "s", "S", "b", "B", "h", "H":
		return "str"
	case "@":
		return "at"
	case "c", "C":
		return "char"
	default:
		return conv
	}
}

// printfAccepts reports whether String.format can apply conv to an argument
// of kind. %s accepts a number. %d does not accept a string.
func printfAccepts(convKind, argKind string) bool {
	if convKind == "str" {
		return argKind == "str" || argKind == "int" || argKind == "float" || argKind == "char"
	}
	return convKind == argKind
}

func printfArgs(text string) []printfArg {
	var out []printfArg
	ordinary := 0
	for _, m := range printfSpec.FindAllStringSubmatch(text, -1) {
		if m[0] == "%%" {
			continue
		}
		var idx int
		var conv string
		if m[1] != "" {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			idx = n
			conv = m[3]
		} else {
			ordinary++
			idx = ordinary
			conv = m[5]
		}
		out = append(out, printfArg{index: idx, kind: printfKind(conv)})
	}
	return out
}

func stringForms(s *mtproto.LangPackString) map[string]string {
	if s.GetPredicateName() != "langPackStringPluralized" {
		return map[string]string{"value": s.GetValue()}
	}
	out := map[string]string{}
	if s.GetZeroValue() != nil {
		out["zero"] = s.GetZeroValue().GetValue()
	}
	if s.GetOneValue() != nil {
		out["one"] = s.GetOneValue().GetValue()
	}
	if s.GetTwoValue() != nil {
		out["two"] = s.GetTwoValue().GetValue()
	}
	if s.GetFewValue() != nil {
		out["few"] = s.GetFewValue().GetValue()
	}
	if s.GetManyValue() != nil {
		out["many"] = s.GetManyValue().GetValue()
	}
	if s.GetOtherValue() != "" {
		out["other"] = s.GetOtherValue()
	}
	return out
}

// englishFormatArgs is the argument list String.format receives for a key.
// Plural forms share that list: a form may omit an argument, and a later form
// may still use it. An int wins over %s when any English form formats that
// index with %d, because that is the value the call passes.
func englishFormatArgs(forms map[string]string) map[int]string {
	seen := map[int]map[string]struct{}{}
	for _, text := range forms {
		for _, arg := range printfArgs(text) {
			if seen[arg.index] == nil {
				seen[arg.index] = map[string]struct{}{}
			}
			seen[arg.index][arg.kind] = struct{}{}
		}
	}
	out := map[int]string{}
	for idx, kinds := range seen {
		switch {
		case hasKind(kinds, "int"):
			out[idx] = "int"
		case hasKind(kinds, "float"):
			out[idx] = "float"
		case hasKind(kinds, "char"):
			out[idx] = "char"
		case hasKind(kinds, "at") && !hasKind(kinds, "str"):
			out[idx] = "at"
		default:
			out[idx] = "str"
		}
	}
	return out
}

func hasKind(kinds map[string]struct{}, kind string) bool {
	_, ok := kinds[kind]
	return ok
}

// TestAndroidPrintfPlaceholdersMatchEnglish fails when a translation adds a
// printf argument English does not pass, or uses a conversion String.format
// would reject for that argument. Omitting an argument is allowed.
func TestAndroidPrintfPlaceholdersMatchEnglish(t *testing.T) {
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	enDiff, err := cat.Difference("android", "en", 0)
	if err != nil {
		t.Fatal(err)
	}
	enBy := map[string]*mtproto.LangPackString{}
	for _, s := range enDiff.GetStrings() {
		enBy[s.GetKey()] = s
	}
	if len(enBy) < 1000 {
		t.Fatalf("english android strings %d", len(enBy))
	}
	codes := make([]string, 0, len(cat.platforms["android"]))
	for code := range cat.platforms["android"] {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	var checked int
	for _, code := range codes {
		if code == "en" {
			continue
		}
		diff, err := cat.Difference("android", code, 0)
		if err != nil {
			t.Fatal(code, err)
		}
		for _, s := range diff.GetStrings() {
			en, ok := enBy[s.GetKey()]
			if !ok {
				continue
			}
			args := englishFormatArgs(stringForms(en))
			for form, text := range stringForms(s) {
				checked++
				for _, arg := range printfArgs(text) {
					kind, have := args[arg.index]
					if !have {
						t.Errorf("%s %s %s adds %%%d (%s) missing from en: %q", code, s.GetKey(), form, arg.index, arg.kind, text)
						continue
					}
					if !printfAccepts(arg.kind, kind) {
						t.Errorf("%s %s %s %% %d is %s, en passes %s: %q", code, s.GetKey(), form, arg.index, arg.kind, kind, text)
					}
				}
			}
		}
	}
	if checked < 1000 {
		t.Fatalf("checked %d strings", checked)
	}
}
