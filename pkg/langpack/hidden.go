package langpack

import (
	"os"
	"strings"
)

// hiddenLangEnv lists codes langpack.getLanguages does not offer.
// Unset uses defaultHiddenLangs. An empty value lists every pack.
// getLangPack, getStrings, getDifference, and getLanguage still serve
// a hidden code so a client that already selected it keeps working.
const hiddenLangEnv = "TELLOMI_LANGPACK_HIDDEN"

// defaultHiddenLangs is Hindi, Thai, and Greek. The official export is still
// partial, and the strings added to match English are machine translations
// (mt:true) waiting for review, so langpack.getLanguages omits them.
// getLangPack still serves the full pack.
const defaultHiddenLangs = "hi,th,el"

// HiddenLang reports whether langpack.getLanguages should omit code.
// A regional tag such as hi-IN counts as the pack it resolves to.
func HiddenLang(code string) bool {
	canon := packCode(code)
	if canon == "" {
		return false
	}
	_, hide := hiddenLangSet()[canon]
	return hide
}

func hiddenLangRaw() string {
	raw, ok := os.LookupEnv(hiddenLangEnv)
	if !ok {
		return defaultHiddenLangs
	}
	return raw
}

func hiddenLangSet() map[string]struct{} {
	raw := hiddenLangRaw()
	set := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		part = packCode(part)
		if part == "" {
			continue
		}
		set[part] = struct{}{}
	}
	return set
}

// packCode is the catalog code for a language tag, or the normalized tag
// when it does not name a pack we ship.
func packCode(code string) string {
	if canon, ok := CanonicalLang(code); ok && canon != "" {
		return canon
	}
	if primary := officialPrimary(code); primary != "" {
		return primary
	}
	return normLang(code)
}
