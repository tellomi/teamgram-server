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
// zh and zh-CN become zh-hans. zh-Hant, zh-TW, zh-HK, and the bare region
// tags TW/HK/MO become zh-hant. classic-zh-cn becomes zh-hans and taiwan
// becomes zh-hant, so a client that still has the removed pack selected
// loads the official strings. English tags become en. Other official
// language codes, including pt_br and iw, map onto the pack code.
// The boolean is false when the tag is not one of those languages.
func CanonicalLang(code string) (string, bool) {
	s := normLang(code)
	if s == "" {
		return "", false
	}
	if to, ok := legacyLang(s); ok {
		return to, true
	}
	if isTraditionalChinese(s) {
		return "zh-hant", true
	}
	if isSimplifiedChinese(s) {
		return "zh-hans", true
	}
	if s == "en" || strings.HasPrefix(s, "en-") {
		return "en", true
	}
	if code, ok := officialCode(s); ok {
		return code, true
	}
	return s, false
}

func isTraditionalChinese(s string) bool {
	if strings.Contains(s, "hant") {
		return true
	}
	switch s {
	case "zh-tw", "zh-hk", "zh-mo", "tw", "hk", "mo":
		return true
	}
	return strings.HasPrefix(s, "zh-tw") || strings.HasPrefix(s, "zh-hk") || strings.HasPrefix(s, "zh-mo")
}

func isSimplifiedChinese(s string) bool {
	switch s {
	case "zh", "zh-cn", "zh-sg", "zh-hans", "cn", "zh-hans-cn":
		return true
	}
	if strings.HasPrefix(s, "zh-hans") || strings.HasPrefix(s, "zh-cn") || strings.HasPrefix(s, "zh-sg") {
		return true
	}
	return false
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

// officialPrimary maps a regional tag such as fr-FR onto the pack code fr.
// Exact tags and the Chinese and English aliases are handled by CanonicalLang.
func officialPrimary(code string) string {
	s := normLang(code)
	i := strings.IndexByte(s, '-')
	if i <= 0 {
		return ""
	}
	if c, ok := officialCode(s[:i]); ok {
		return c
	}
	return ""
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
