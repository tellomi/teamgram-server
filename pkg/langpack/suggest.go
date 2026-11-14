// Copyright 2026 Tellomi Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0

package langpack

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
	"golang.org/x/text/language"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func normLang(code string) string {
	s := strings.ToLower(strings.TrimSpace(code))
	return strings.ReplaceAll(s, "_", "-")
}

// legacyLang maps a removed duplicate pack onto the official pack that
// replaces it. Those codes are not listed by langpack.getLanguages.
// classic-zh-cn is the old community 简体中文 pack. taiwan is 正體中文.
func legacyLang(code string) (string, bool) {
	switch normLang(code) {
	case "classic-zh-cn":
		return "zh-hans", true
	case "taiwan":
		return "zh-hant", true
	default:
		return "", false
	}
}

// CanonicalLang maps a client or system language tag onto a pack we serve.
// zh and zh-CN become zh-hans. zh-Hant, zh-TW, zh-HK, zh-MO, the Android
// resource tags zh-rTW / zh-rHK / zh-rMO, zh-cht, cmn-Hant, and yue-HK
// become zh-hant. zh-Hans, zh-SG, zh-rCN, zh-chs, and cmn-Hans become
// zh-hans. classic-zh-cn becomes zh-hans and taiwan becomes zh-hant, so a
// client that still has the removed pack selected loads the official strings.
// English tags become en. A regional tag such as ja-JP, fr-FR, or de_DE
// becomes the pack we ship (ja, fr, de). The legacy Android tag in becomes
// id. The boolean is false when the tag is not one of those languages, so
// the caller can try the next field before English.
func CanonicalLang(code string) (string, bool) {
	s := normLang(code)
	if s == "" {
		return "", false
	}
	if to, ok := legacyLang(s); ok {
		return to, true
	}
	if to, ok := chinesePack(s); ok {
		return to, true
	}
	if s == "en" || strings.HasPrefix(s, "en-") {
		return "en", true
	}
	if code, ok := officialCode(s); ok {
		return code, true
	}
	if primary := officialPrimary(s); primary != "" {
		return primary, true
	}
	return "", false
}

// chinesePack matches simplified and traditional Chinese, including the
// script, region, and Android resource forms clients actually send.
func chinesePack(s string) (string, bool) {
	s = stripPrivate(s)
	if s == "" {
		return "", false
	}
	switch s {
	case "zh", "zh-cn", "zh-sg", "zh-my", "zh-hans", "cn", "zh-hans-cn", "zh-chs", "chs", "hans":
		return "zh-hans", true
	case "zh-tw", "zh-hk", "zh-mo", "zh-hant", "tw", "hk", "mo", "zh-cht", "cht", "hant":
		return "zh-hant", true
	}
	if strings.HasPrefix(s, "zh-hans") || strings.HasPrefix(s, "zh-cn") || strings.HasPrefix(s, "zh-sg") || strings.HasPrefix(s, "zh-chs") {
		return "zh-hans", true
	}
	if strings.HasPrefix(s, "zh-hant") || strings.HasPrefix(s, "zh-tw") || strings.HasPrefix(s, "zh-hk") || strings.HasPrefix(s, "zh-mo") || strings.HasPrefix(s, "zh-cht") {
		return "zh-hant", true
	}
	// Android resource qualifiers: zh-rCN, zh-rTW, zh-rHK, zh-rMO, zh-rSG.
	if strings.HasPrefix(s, "zh-r") && len(s) >= 6 {
		switch s[4:6] {
		case "tw", "hk", "mo":
			return "zh-hant", true
		case "cn", "sg", "my":
			return "zh-hans", true
		}
	}
	tag, err := language.Parse(s)
	if err != nil && tag.IsRoot() {
		return "", false
	}
	base, _ := tag.Base()
	switch base.String() {
	case "zh", "cmn", "yue":
	default:
		return "", false
	}
	script, _ := tag.Script()
	switch script.String() {
	case "Hant":
		return "zh-hant", true
	case "Hans":
		return "zh-hans", true
	case "Latn":
		return "", false
	}
	region, _ := tag.Region()
	switch region.String() {
	case "TW", "HK", "MO":
		return "zh-hant", true
	case "CN", "SG", "MY":
		return "zh-hans", true
	}
	if base.String() == "yue" {
		return "zh-hant", true
	}
	return "zh-hans", true
}

func stripPrivate(s string) string {
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = strings.TrimRight(s[:i], "-")
	}
	return s
}

// SuggestLangCode is the value help.getConfig should return in suggested_lang_code.
// Official clients send the device language as initConnection.system_lang_code
// and the in-app language as lang_code. The suggestion follows the device
// language. An empty, unknown, or hidden device language falls back to
// lang_code. A hidden pack is never suggested. Anything else is en, which
// keeps config's shared flag bit for suggested_lang_code / lang_pack_version valid.
func SuggestLangCode(langCode, systemLangCode string) string {
	if code, ok := suggestable(systemLangCode); ok {
		return code
	}
	if code, ok := suggestable(langCode); ok {
		return code
	}
	return "en"
}

func suggestable(code string) (string, bool) {
	canon := packCode(code)
	if canon == "" || HiddenLang(canon) {
		return "", false
	}
	if _, ok := CanonicalLang(canon); !ok {
		return "", false
	}
	return canon, true
}

// officialPrimary maps a regional tag such as fr-FR or no-NO onto the pack
// code. The tag has to parse: a junk suffix such as no-such is not Norwegian.
// Exact tags and the Chinese and English aliases are handled by CanonicalLang.
func officialPrimary(code string) string {
	s := normLang(code)
	tag, err := language.Parse(s)
	if err != nil {
		return ""
	}
	base, bconf := tag.Base()
	if bconf == language.No {
		return ""
	}
	_, sconf := tag.Script()
	_, rconf := tag.Region()
	if sconf == language.No && rconf == language.No {
		return ""
	}
	if c, ok := officialCode(base.String()); ok {
		return c
	}
	return ""
}

// ClientLang is the language for text this server writes to a client.
// initConnection.lang_code wins, then lang_pack when that field is itself
// a language tag, then system_lang_code. lang_pack is usually a platform
// name such as "android" or "ios", and those values are skipped. A regional
// tag is matched onto the pack before the next field is tried. English is
// used only when none of the three names a pack we serve.
func ClientLang(langCode, systemLangCode, langPack string) string {
	for _, raw := range []string{langCode, langPack, systemLangCode} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if code, ok := CanonicalLang(raw); ok && code != "" {
			return code
		}
	}
	return "en"
}

// NormalizePlatform maps client lang_pack names onto catalog directories.
func NormalizePlatform(pack string) string {
	switch strings.ToLower(strings.TrimSpace(pack)) {
	case "android", "android_x", "androidx":
		return "android"
	case "ios":
		return "ios"
	case "tdesktop", "desktop":
		return "tdesktop"
	case "macos", "macosx", "osx", "mac":
		return "macos"
	default:
		return strings.ToLower(strings.TrimSpace(pack))
	}
}

// ApplyConfigLanguage sets suggested_lang_code from the initConnection languages
// and aligns lang_pack_version with the pack the client is currently using.
func ApplyConfigLanguage(cfg *mtproto.TLConfig, langCode, systemLangCode, langPack string) {
	if cfg == nil {
		return
	}
	cfg.SetSuggestedLangCode(&wrapperspb.StringValue{Value: SuggestLangCode(langCode, systemLangCode)})
	cat := DefaultCatalog()
	if cat == nil {
		return
	}
	pack := NormalizePlatform(langPack)
	if !cat.HasPlatform(pack) {
		switch {
		case cat.HasPlatform("android"):
			pack = "android"
		case cat.HasPlatform("ios"):
			pack = "ios"
		default:
			return
		}
	}
	current, ok := CanonicalLang(langCode)
	if !ok || !cat.HasLang(pack, current) {
		current = "en"
	}
	if v, ok := cat.Version(pack, current); ok {
		cfg.SetLangPackVersion(&wrapperspb.Int32Value{Value: v})
	}
	base := "en"
	if b, ok := cat.BaseLang(pack, current); ok && b != "" {
		base = b
	}
	if v, ok := cat.Version(pack, base); ok {
		cfg.SetBaseLangPackVersion(&wrapperspb.Int32Value{Value: v})
	}
}
