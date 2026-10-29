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

// CanonicalLang maps a client or system language tag onto a pack we serve.
// zh and zh-CN become zh-hans. zh-Hant, zh-TW, zh-HK, and the bare region
// tags TW/HK/MO become zh-hant. English tags become en.
// The boolean is false when the tag is not one of those languages.
func CanonicalLang(code string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(code))
	s = strings.ReplaceAll(s, "_", "-")
	if s == "" {
		return "", false
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
// The device language wins when it is Chinese, so a phone set to 简体中文 or
// 繁體中文 is offered that pack even while the app language is still en.
// Otherwise the app lang_code from initConnection is used. Anything else is en,
// which keeps config's shared flag bit for suggested_lang_code / lang_pack_version valid.
func SuggestLangCode(langCode, systemLangCode string) string {
	if sys, ok := CanonicalLang(systemLangCode); ok && (sys == "zh-hans" || sys == "zh-hant") {
		return sys
	}
	if lang, ok := CanonicalLang(langCode); ok && (lang == "zh-hans" || lang == "zh-hant") {
		return lang
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
