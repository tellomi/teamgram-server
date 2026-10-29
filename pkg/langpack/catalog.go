// Copyright 2026 Tellomi Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0

package langpack

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	langpackdata "github.com/teamgram/teamgram-server/data/langpack"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	ErrUnknownPack = errors.New("lang pack invalid")
	ErrUnknownLang = errors.New("lang code not supported")
)

// languageOrder is the order clients see in langpack.getLanguages.
// Chinese is first because Tellomi launches in mainland China.
var languageOrder = []string{"zh-hans", "zh-hant", "en"}

// Entry is one langpack string. Plural is set for langPackStringPluralized.
type Entry struct {
	Key     string
	Value   string
	Plural  *Plural
	Version int32
}

// Plural holds CLDR forms. Empty means the form is absent.
type Plural struct {
	Zero  string
	One   string
	Two   string
	Few   string
	Many  string
	Other string
}

func (p *Plural) form(name string) string {
	if p == nil {
		return ""
	}
	switch name {
	case "zero":
		return p.Zero
	case "one":
		return p.One
	case "two":
		return p.Two
	case "few":
		return p.Few
	case "many":
		return p.Many
	case "other":
		return p.Other
	default:
		return ""
	}
}

func (p *Plural) any() bool {
	return p != nil && (p.Zero != "" || p.One != "" || p.Two != "" || p.Few != "" || p.Many != "" || p.Other != "")
}

// Language is one platform's pack.
type Language struct {
	Code            string
	Name            string
	NativeName      string
	PluralCode      string
	Official        bool
	RTL             bool
	Beta            bool
	BaseLangCode    string
	TranslationsURL string
	Version         int32
	Strings         []Entry
}

type loaded struct {
	meta   Language
	byKey  map[string]Entry
	plural map[string]Entry // suffix key -> plural entry (base key in Entry.Key)
}

// Catalog is every platform pack loaded at startup.
type Catalog struct {
	platforms map[string]map[string]*loaded
}

// NewCatalog builds a catalog from already-decoded languages.
// platforms maps platform -> lang code -> language.
func NewCatalog(platforms map[string]map[string]*Language) *Catalog {
	c := &Catalog{platforms: map[string]map[string]*loaded{}}
	for platform, langs := range platforms {
		platform = NormalizePlatform(platform)
		dst := map[string]*loaded{}
		for code, lang := range langs {
			if lang == nil {
				continue
			}
			dst[code] = indexLanguage(platform, lang)
		}
		if len(dst) > 0 {
			c.platforms[platform] = dst
		}
	}
	return c
}

func indexLanguage(platform string, lang *Language) *loaded {
	l := &loaded{
		meta:   *lang,
		byKey:  map[string]Entry{},
		plural: map[string]Entry{},
	}
	maxV := lang.Version
	for _, e := range lang.Strings {
		if e.Version <= 0 {
			e.Version = 1
		}
		if e.Version > maxV {
			maxV = e.Version
		}
		l.byKey[e.Key] = e
		if e.Plural != nil && e.Plural.any() {
			for _, suf := range suffixTable(platform) {
				if e.Plural.form(suf.form) == "" {
					continue
				}
				l.plural[e.Key+suf.suffix] = e
			}
		}
	}
	if l.meta.Version < maxV {
		l.meta.Version = maxV
	}
	if l.meta.Version <= 0 {
		l.meta.Version = 1
	}
	return l
}

type suffix struct {
	suffix string
	form   string
}

func suffixTable(platform string) []suffix {
	if platform == "ios" || platform == "macos" {
		return []suffix{
			{"_3_10", "few"},
			{"_many", "many"},
			{"_any", "other"},
			{"_0", "zero"},
			{"_1", "one"},
			{"_2", "two"},
		}
	}
	return []suffix{
		{"_zero", "zero"},
		{"_one", "one"},
		{"_two", "two"},
		{"_few", "few"},
		{"_many", "many"},
		{"_other", "other"},
	}
}

func (l *loaded) stringsCount(base *loaded) int32 {
	n := len(l.byKey)
	if base != nil && len(base.byKey) > n {
		n = len(base.byKey)
	}
	return int32(n)
}

func (l *loaded) translatedCount() int32 {
	return int32(len(l.byKey))
}

// HasPlatform reports whether any language is loaded for the platform.
func (c *Catalog) HasPlatform(platform string) bool {
	if c == nil {
		return false
	}
	_, ok := c.platforms[NormalizePlatform(platform)]
	return ok
}

// HasLang reports whether the language exists on the platform.
func (c *Catalog) HasLang(platform, code string) bool {
	_, ok := c.lookup(platform, code)
	return ok
}

func (c *Catalog) lookup(platform, code string) (*loaded, bool) {
	if c == nil {
		return nil, false
	}
	langs := c.platforms[NormalizePlatform(platform)]
	if langs == nil {
		return nil, false
	}
	canon, ok := CanonicalLang(code)
	if !ok {
		canon = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), "_", "-"))
	}
	l, ok := langs[canon]
	return l, ok
}

// Version is the pack version for getDifference and help.getConfig.
func (c *Catalog) Version(platform, code string) (int32, bool) {
	l, ok := c.lookup(platform, code)
	if !ok {
		return 0, false
	}
	return l.meta.Version, true
}

// BaseLang is the language's base_lang_code, if any.
func (c *Catalog) BaseLang(platform, code string) (string, bool) {
	l, ok := c.lookup(platform, code)
	if !ok {
		return "", false
	}
	return l.meta.BaseLangCode, true
}

// Languages implements langpack.getLanguages.
func (c *Catalog) Languages(platform string) ([]*mtproto.LangPackLanguage, error) {
	if c == nil || !c.HasPlatform(platform) {
		return nil, ErrUnknownPack
	}
	langs := c.platforms[NormalizePlatform(platform)]
	var codes []string
	seen := map[string]bool{}
	for _, code := range languageOrder {
		if _, ok := langs[code]; ok {
			codes = append(codes, code)
			seen[code] = true
		}
	}
	var rest []string
	for code := range langs {
		if !seen[code] {
			rest = append(rest, code)
		}
	}
	sort.Strings(rest)
	codes = append(codes, rest...)
	out := make([]*mtproto.LangPackLanguage, 0, len(codes))
	for _, code := range codes {
		out = append(out, c.languageInfo(platform, langs[code]))
	}
	return out, nil
}

// Language implements langpack.getLanguage.
func (c *Catalog) Language(platform, code string) (*mtproto.LangPackLanguage, error) {
	if c == nil || !c.HasPlatform(platform) {
		return nil, ErrUnknownPack
	}
	l, ok := c.lookup(platform, code)
	if !ok {
		return nil, ErrUnknownLang
	}
	return c.languageInfo(platform, l), nil
}

func (c *Catalog) languageInfo(platform string, l *loaded) *mtproto.LangPackLanguage {
	var base *loaded
	if l.meta.BaseLangCode != "" {
		base, _ = c.lookup(platform, l.meta.BaseLangCode)
	}
	info := &mtproto.LangPackLanguage{
		Official:        l.meta.Official,
		Rtl:             l.meta.RTL,
		Beta:            l.meta.Beta,
		Name:            l.meta.Name,
		NativeName:      l.meta.NativeName,
		LangCode:        l.meta.Code,
		PluralCode:      l.meta.PluralCode,
		StringsCount:    l.stringsCount(base),
		TranslatedCount: l.translatedCount(),
		TranslationsUrl: l.meta.TranslationsURL,
	}
	if l.meta.BaseLangCode != "" {
		info.BaseLangCode = &wrapperspb.StringValue{Value: l.meta.BaseLangCode}
	}
	return mtproto.MakeTLLangPackLanguage(info).To_LangPackLanguage()
}

// Difference implements langpack.getLangPack (fromVersion 0) and getDifference.
func (c *Catalog) Difference(platform, code string, fromVersion int32) (*mtproto.LangPackDifference, error) {
	if c == nil || !c.HasPlatform(platform) {
		return nil, ErrUnknownPack
	}
	l, ok := c.lookup(platform, code)
	if !ok {
		return nil, ErrUnknownLang
	}
	if fromVersion < 0 {
		fromVersion = 0
	}
	platform = NormalizePlatform(platform)
	var strings []*mtproto.LangPackString
	for _, e := range l.meta.Strings {
		ver := e.Version
		if ver <= 0 {
			ver = 1
		}
		if ver <= fromVersion {
			continue
		}
		strings = append(strings, e.mtproto(platform)...)
	}
	if strings == nil {
		strings = []*mtproto.LangPackString{}
	}
	return mtproto.MakeTLLangPackDifference(&mtproto.LangPackDifference{
		LangCode:    l.meta.Code,
		FromVersion: fromVersion,
		Version:     l.meta.Version,
		Strings:     strings,
	}).To_LangPackDifference(), nil
}

// Strings implements langpack.getStrings.
func (c *Catalog) Strings(platform, code string, keys []string) ([]*mtproto.LangPackString, error) {
	if c == nil || !c.HasPlatform(platform) {
		return nil, ErrUnknownPack
	}
	l, ok := c.lookup(platform, code)
	if !ok {
		return nil, ErrUnknownLang
	}
	platform = NormalizePlatform(platform)
	out := make([]*mtproto.LangPackString, 0, len(keys))
	for _, key := range keys {
		if e, ok := l.byKey[key]; ok {
			out = append(out, e.mtproto(platform)...)
			continue
		}
		if e, ok := l.plural[key]; ok {
			val := pluralValueForKey(e, key, platform)
			if val == "" {
				val = e.Plural.Other
			}
			out = append(out, plainString(key, val))
			continue
		}
		out = append(out, mtproto.MakeTLLangPackStringDeleted(&mtproto.LangPackString{Key: key}).To_LangPackString())
	}
	return out, nil
}

func pluralValueForKey(e Entry, key, platform string) string {
	for _, suf := range suffixTable(platform) {
		if strings.HasSuffix(key, suf.suffix) && strings.TrimSuffix(key, suf.suffix) == e.Key {
			return e.Plural.form(suf.form)
		}
	}
	return ""
}

func (e Entry) mtproto(platform string) []*mtproto.LangPackString {
	if e.Plural == nil || !e.Plural.any() {
		return []*mtproto.LangPackString{plainString(e.Key, e.Value)}
	}
	p := e.Plural
	raw := &mtproto.LangPackString{Key: e.Key, OtherValue: p.Other}
	if p.Zero != "" {
		raw.ZeroValue = &wrapperspb.StringValue{Value: p.Zero}
	}
	if p.One != "" {
		raw.OneValue = &wrapperspb.StringValue{Value: p.One}
	}
	if p.Two != "" {
		raw.TwoValue = &wrapperspb.StringValue{Value: p.Two}
	}
	if p.Few != "" {
		raw.FewValue = &wrapperspb.StringValue{Value: p.Few}
	}
	if p.Many != "" {
		raw.ManyValue = &wrapperspb.StringValue{Value: p.Many}
	}
	out := []*mtproto.LangPackString{mtproto.MakeTLLangPackStringPluralized(raw).To_LangPackString()}
	// iOS looks up the zero form as key+"_0". dictFromLocalization stores the
	// pluralized zero form under key+"_zero", so also send the suffixed string.
	if (platform == "ios" || platform == "macos") && p.Zero != "" {
		out = append(out, plainString(e.Key+"_0", p.Zero))
	}
	return out
}

func plainString(key, value string) *mtproto.LangPackString {
	return mtproto.MakeTLLangPackString(&mtproto.LangPackString{Key: key, Value: value}).To_LangPackString()
}

type filePack struct {
	LangCode        string       `json:"lang_code"`
	Name            string       `json:"name"`
	NativeName      string       `json:"native_name"`
	PluralCode      string       `json:"plural_code"`
	Official        bool         `json:"official"`
	RTL             bool         `json:"rtl"`
	Beta            bool         `json:"beta"`
	BaseLangCode    string       `json:"base_lang_code"`
	TranslationsURL string       `json:"translations_url"`
	Version         int32        `json:"version"`
	Strings         []fileString `json:"strings"`
}

type fileString struct {
	Key     string  `json:"key"`
	Value   string  `json:"value,omitempty"`
	Zero    *string `json:"zero,omitempty"`
	One     *string `json:"one,omitempty"`
	Two     *string `json:"two,omitempty"`
	Few     *string `json:"few,omitempty"`
	Many    *string `json:"many,omitempty"`
	Other   *string `json:"other,omitempty"`
	Version int32   `json:"version,omitempty"`
}

func (fs fileString) plural() *Plural {
	if fs.Zero == nil && fs.One == nil && fs.Two == nil && fs.Few == nil && fs.Many == nil && fs.Other == nil {
		return nil
	}
	p := &Plural{}
	if fs.Zero != nil {
		p.Zero = *fs.Zero
	}
	if fs.One != nil {
		p.One = *fs.One
	}
	if fs.Two != nil {
		p.Two = *fs.Two
	}
	if fs.Few != nil {
		p.Few = *fs.Few
	}
	if fs.Many != nil {
		p.Many = *fs.Many
	}
	if fs.Other != nil {
		p.Other = *fs.Other
	}
	if !p.any() {
		return nil
	}
	return p
}

// Load reads platform directories of <lang>.json from fsys.
func Load(fsys fs.FS) (*Catalog, error) {
	platforms := map[string]map[string]*Language{}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		platform := NormalizePlatform(ent.Name())
		files, err := fs.ReadDir(fsys, ent.Name())
		if err != nil {
			return nil, err
		}
		langs := map[string]*Language{}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			body, err := fs.ReadFile(fsys, path.Join(ent.Name(), f.Name()))
			if err != nil {
				return nil, err
			}
			lang, err := decodeLanguage(body)
			if err != nil {
				return nil, err
			}
			langs[lang.Code] = lang
		}
		if len(langs) > 0 {
			platforms[platform] = langs
		}
	}
	return NewCatalog(platforms), nil
}

func decodeLanguage(body []byte) (*Language, error) {
	var raw filePack
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	lang := &Language{
		Code:            raw.LangCode,
		Name:            raw.Name,
		NativeName:      raw.NativeName,
		PluralCode:      raw.PluralCode,
		Official:        raw.Official,
		RTL:             raw.RTL,
		Beta:            raw.Beta,
		BaseLangCode:    raw.BaseLangCode,
		TranslationsURL: raw.TranslationsURL,
		Version:         raw.Version,
	}
	if canon, ok := CanonicalLang(lang.Code); ok {
		lang.Code = canon
	}
	for _, s := range raw.Strings {
		e := Entry{Key: s.Key, Value: s.Value, Version: s.Version, Plural: s.plural()}
		if e.Plural != nil {
			e.Value = ""
		}
		lang.Strings = append(lang.Strings, e)
	}
	return lang, nil
}

var (
	defaultOnce sync.Once
	defaultCat  *Catalog
	defaultErr  error
)

// DefaultCatalog is the pack shipped with the server.
// TELLOMI_LANGPACK_DIR replaces the embedded files when it is set.
func DefaultCatalog() *Catalog {
	defaultOnce.Do(func() {
		if dir := strings.TrimSpace(os.Getenv("TELLOMI_LANGPACK_DIR")); dir != "" {
			defaultCat, defaultErr = Load(os.DirFS(dir))
			return
		}
		defaultCat, defaultErr = Load(langpackdata.FS)
	})
	return defaultCat
}

// DefaultError is the error from loading DefaultCatalog, if any.
func DefaultError() error {
	DefaultCatalog()
	return defaultErr
}
