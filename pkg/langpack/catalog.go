// Copyright 2026 Tellomi Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0

package langpack

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	langpackdata "github.com/teamgram/teamgram-server/data/langpack"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	ErrUnknownPack    = errors.New("lang pack invalid")
	ErrUnknownLang    = errors.New("lang code not supported")
	errSkipLegacyPack = errors.New("skip legacy lang pack")
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
	// AndroidXMLFixVersion is the first Android pack version whose strings
	// survive stock Android's XML writer. Zero means this pack has no such
	// cutover. A client still below it receives the whole pack.
	AndroidXMLFixVersion int32
	Strings              []Entry
}

type loaded struct {
	meta   Language
	byKey  map[string]Entry
	plural map[string]Entry // suffix key -> plural entry (base key in Entry.Key)
}

// Catalog is every platform pack loaded at startup.
type Catalog struct {
	platforms       map[string]map[string]*loaded
	androidXMLDrops map[string]int

	// built stores one protobuf pack per (lang_pack, lang_code, version).
	// A version change misses and the previous entry is dropped. Callers
	// must not mutate the returned objects.
	diffMu   sync.Mutex
	built    map[builtKey]*builtPack
	building map[builtKey]*builtFlight
	partial  map[diffKey]*mtproto.LangPackDifference
	langs    map[langKey][]*mtproto.LangPackLanguage
}

type builtKey struct {
	platform string
	code     string
	version  int32
}

type diffKey struct {
	platform string
	code     string
	from     int32
	version  int32
}

type langKey struct {
	platform string
	hidden   string
	stamp    string
}

type builtItem struct {
	version int32
	parts   []*mtproto.LangPackString
}

type builtPack struct {
	full  *mtproto.LangPackDifference
	items []builtItem
	byKey map[string][]*mtproto.LangPackString
}

type builtFlight struct {
	ch   chan struct{}
	pack *builtPack
}

// NewCatalog builds a catalog from already-decoded languages.
// platforms maps platform -> lang code -> language.
func NewCatalog(platforms map[string]map[string]*Language) *Catalog {
	c := &Catalog{
		platforms:       map[string]map[string]*loaded{},
		androidXMLDrops: map[string]int{},
	}
	for platform, langs := range platforms {
		platform = NormalizePlatform(platform)
		dst := map[string]*loaded{}
		for code, lang := range langs {
			if lang == nil {
				continue
			}
			loadedLang, dropped := indexLanguage(platform, lang)
			dst[code] = loadedLang
			if platform == "android" {
				c.androidXMLDrops[code] = dropped
			}
		}
		if len(dst) > 0 {
			c.platforms[platform] = dst
		}
	}
	return c
}

func indexLanguage(platform string, lang *Language) (*loaded, int) {
	l := &loaded{
		meta:   *lang,
		byKey:  map[string]Entry{},
		plural: map[string]Entry{},
	}
	kept := make([]Entry, 0, len(lang.Strings))
	dropped := 0
	maxV := lang.Version
	for _, e := range lang.Strings {
		if platform == "android" && !androidEntrySafe(e) {
			dropped++
			continue
		}
		if e.Version <= 0 {
			e.Version = 1
		}
		if e.Version > maxV {
			maxV = e.Version
		}
		kept = append(kept, e)
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
	l.meta.Strings = kept
	if l.meta.Version < maxV {
		l.meta.Version = maxV
	}
	if l.meta.Version <= 0 {
		l.meta.Version = 1
	}
	return l, dropped
}

type suffix struct {
	suffix string
	form   string
}

func suffixTable(platform string) []suffix {
	ios := []suffix{
		{"_3_10", "few"},
		{"_many", "many"},
		{"_any", "other"},
		{"_0", "zero"},
		{"_1", "one"},
		{"_2", "two"},
	}
	android := []suffix{
		{"_zero", "zero"},
		{"_one", "one"},
		{"_two", "two"},
		{"_few", "few"},
		{"_many", "many"},
		{"_other", "other"},
	}
	switch platform {
	case "ios":
		return ios
	case "macos":
		// The macOS export uses both iOS suffixes (_1, _any) and Android suffixes (_one, _other).
		return append(append([]suffix{}, ios...), android...)
	case "tdesktop":
		// Desktop plural keys are lng_key#one. A few keys still use _one.
		return append([]suffix{
			{"#zero", "zero"},
			{"#one", "one"},
			{"#two", "two"},
			{"#few", "few"},
			{"#many", "many"},
			{"#other", "other"},
		}, android...)
	default:
		return android
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
	// Settings and the phone locale send regional tags (de-DE, fr-FR,
	// ja-JP, en-US, zh_Hans). iOS aborts the whole language switch when
	// getLanguage returns an error. The difference still reports the
	// catalog code (de, not de-DE): that is the code getLanguages listed,
	// and both clients compare the response against it.
	l, ok := langs[packCode(code)]
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

// Plain is one string value. A missing key follows base_lang_code.
// Every pack except English uses en, so the walk ends at English.
func (c *Catalog) Plain(platform, code, key string) (string, bool) {
	seen := map[string]struct{}{}
	for code != "" {
		if _, ok := seen[code]; ok {
			return "", false
		}
		seen[code] = struct{}{}
		l, ok := c.lookup(platform, code)
		if !ok {
			return "", false
		}
		if e, ok := l.byKey[key]; ok && e.Value != "" {
			return e.Value, true
		}
		base := strings.TrimSpace(l.meta.BaseLangCode)
		if base == "" || base == code {
			return "", false
		}
		code = base
	}
	return "", false
}

// Languages implements langpack.getLanguages.
func (c *Catalog) Languages(platform string) ([]*mtproto.LangPackLanguage, error) {
	if c == nil || !c.HasPlatform(platform) {
		return nil, ErrUnknownPack
	}
	platform = NormalizePlatform(platform)
	key := langKey{platform: platform, hidden: hiddenLangRaw(), stamp: c.versionStamp(platform)}
	c.diffMu.Lock()
	if hit := c.langs[key]; hit != nil {
		c.diffMu.Unlock()
		return append([]*mtproto.LangPackLanguage(nil), hit...), nil
	}
	c.diffMu.Unlock()
	langs := c.platforms[platform]
	hidden := hiddenLangSet()
	var codes []string
	seen := map[string]bool{}
	for _, code := range languageOrder {
		if _, hide := hidden[code]; hide {
			continue
		}
		if _, ok := langs[code]; ok {
			codes = append(codes, code)
			seen[code] = true
		}
	}
	var rest []string
	for code := range langs {
		if seen[code] {
			continue
		}
		// A catalog that still has a removed duplicate (classic-zh-cn, taiwan)
		// must not show a second Chinese entry. lookup serves the official pack.
		if _, legacy := legacyLang(code); legacy {
			continue
		}
		if _, hide := hidden[code]; hide {
			continue
		}
		rest = append(rest, code)
	}
	sort.Strings(rest)
	codes = append(codes, rest...)
	out := make([]*mtproto.LangPackLanguage, 0, len(codes))
	for _, code := range codes {
		out = append(out, c.languageInfo(platform, langs[code]))
	}
	c.diffMu.Lock()
	if c.langs == nil {
		c.langs = map[langKey][]*mtproto.LangPackLanguage{}
	}
	c.langs[key] = out
	c.diffMu.Unlock()
	return append([]*mtproto.LangPackLanguage(nil), out...), nil
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
	// Stock Android writes the difference into an XML file and, on the next
	// launch, merges the next difference into that file. A file that failed
	// to parse comes back as an empty map, so a partial difference would
	// replace the whole language with a handful of strings. from_version 0
	// is the client's signal to replace the file. Clients already at the
	// fix version keep the normal incremental difference.
	if platform == "android" && fromVersion > 0 && l.meta.AndroidXMLFixVersion > 0 && fromVersion < l.meta.AndroidXMLFixVersion {
		fromVersion = 0
	}
	pack := c.ensureBuilt(platform, l)
	if fromVersion <= 0 {
		return pack.full, nil
	}
	if fromVersion >= l.meta.Version {
		return mtproto.MakeTLLangPackDifference(&mtproto.LangPackDifference{
			LangCode:    l.meta.Code,
			FromVersion: fromVersion,
			Version:     l.meta.Version,
			Strings:     []*mtproto.LangPackString{},
		}).To_LangPackDifference(), nil
	}
	key := diffKey{platform: platform, code: l.meta.Code, from: fromVersion, version: l.meta.Version}
	c.diffMu.Lock()
	if hit := c.partial[key]; hit != nil {
		c.diffMu.Unlock()
		return hit, nil
	}
	c.diffMu.Unlock()
	var strings []*mtproto.LangPackString
	for _, item := range pack.items {
		if item.version <= fromVersion {
			continue
		}
		strings = append(strings, item.parts...)
	}
	if strings == nil {
		strings = []*mtproto.LangPackString{}
	}
	diff := mtproto.MakeTLLangPackDifference(&mtproto.LangPackDifference{
		LangCode:    l.meta.Code,
		FromVersion: fromVersion,
		Version:     l.meta.Version,
		Strings:     strings,
	}).To_LangPackDifference()
	c.diffMu.Lock()
	if c.partial == nil {
		c.partial = map[diffKey]*mtproto.LangPackDifference{}
	}
	if hit := c.partial[key]; hit != nil {
		c.diffMu.Unlock()
		return hit, nil
	}
	c.partial[key] = diff
	c.diffMu.Unlock()
	return diff, nil
}

// ensureBuilt returns the protobuf pack for this version. Concurrent callers
// share one build. A different version replaces the previous pack.
func (c *Catalog) ensureBuilt(platform string, l *loaded) *builtPack {
	key := builtKey{platform: platform, code: l.meta.Code, version: l.meta.Version}
	c.diffMu.Lock()
	if c.built == nil {
		c.built = map[builtKey]*builtPack{}
		c.building = map[builtKey]*builtFlight{}
	}
	if hit := c.built[key]; hit != nil {
		c.diffMu.Unlock()
		return hit
	}
	if flight := c.building[key]; flight != nil {
		c.diffMu.Unlock()
		<-flight.ch
		return flight.pack
	}
	flight := &builtFlight{ch: make(chan struct{})}
	c.building[key] = flight
	c.diffMu.Unlock()

	pack := buildPack(platform, l)

	c.diffMu.Lock()
	for old := range c.built {
		if old.platform == key.platform && old.code == key.code && old.version != key.version {
			delete(c.built, old)
		}
	}
	for old := range c.partial {
		if old.platform == key.platform && old.code == key.code && old.version != key.version {
			delete(c.partial, old)
		}
	}
	c.built[key] = pack
	delete(c.building, key)
	flight.pack = pack
	close(flight.ch)
	c.diffMu.Unlock()
	return pack
}

func buildPack(platform string, l *loaded) *builtPack {
	items := make([]builtItem, 0, len(l.meta.Strings))
	byKey := make(map[string][]*mtproto.LangPackString, len(l.byKey)+len(l.plural))
	var all []*mtproto.LangPackString
	for _, e := range l.meta.Strings {
		ver := e.Version
		if ver <= 0 {
			ver = 1
		}
		parts := e.mtproto(platform)
		items = append(items, builtItem{version: ver, parts: parts})
		all = append(all, parts...)
	}
	for key, e := range l.byKey {
		byKey[key] = e.mtproto(platform)
	}
	for key, e := range l.plural {
		if _, ok := byKey[key]; ok {
			continue
		}
		val := pluralValueForKey(e, key, platform)
		if val == "" && e.Plural != nil {
			val = e.Plural.Other
		}
		byKey[key] = []*mtproto.LangPackString{plainString(key, val)}
	}
	if all == nil {
		all = []*mtproto.LangPackString{}
	}
	return &builtPack{
		full: mtproto.MakeTLLangPackDifference(&mtproto.LangPackDifference{
			LangCode:    l.meta.Code,
			FromVersion: 0,
			Version:     l.meta.Version,
			Strings:     all,
		}).To_LangPackDifference(),
		items: items,
		byKey: byKey,
	}
}

func (c *Catalog) versionStamp(platform string) string {
	langs := c.platforms[NormalizePlatform(platform)]
	codes := make([]string, 0, len(langs))
	for code := range langs {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	var b strings.Builder
	for _, code := range codes {
		b.WriteString(code)
		b.WriteByte('=')
		b.WriteString(strconv.FormatInt(int64(langs[code].meta.Version), 10))
		b.WriteByte(';')
	}
	return b.String()
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
	pack := c.ensureBuilt(platform, l)
	out := make([]*mtproto.LangPackString, 0, len(keys))
	for _, key := range keys {
		if parts, ok := pack.byKey[key]; ok {
			out = append(out, parts...)
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
	LangCode             string       `json:"lang_code"`
	Name                 string       `json:"name"`
	NativeName           string       `json:"native_name"`
	PluralCode           string       `json:"plural_code"`
	Official             bool         `json:"official"`
	RTL                  bool         `json:"rtl"`
	Beta                 bool         `json:"beta"`
	BaseLangCode         string       `json:"base_lang_code"`
	TranslationsURL      string       `json:"translations_url"`
	Version              int32        `json:"version"`
	AndroidXMLFixVersion int32        `json:"android_xml_fix_version,omitempty"`
	Strings              []fileString `json:"strings"`
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

func rewritePlural(p *Plural) *Plural {
	if p == nil {
		return nil
	}
	p.Zero = brand.Rewrite(p.Zero)
	p.One = brand.Rewrite(p.One)
	p.Two = brand.Rewrite(p.Two)
	p.Few = brand.Rewrite(p.Few)
	p.Many = brand.Rewrite(p.Many)
	p.Other = brand.Rewrite(p.Other)
	return p
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

// Load reads platform directories of <lang>.json or <lang>.json.gz from fsys.
// overrides/ and other non-platform directories are ignored.
func Load(fsys fs.FS) (*Catalog, error) {
	platforms := map[string]map[string]*Language{}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	for _, ent := range entries {
		if !ent.IsDir() || !knownPlatformDir(ent.Name()) {
			continue
		}
		platform := NormalizePlatform(ent.Name())
		files, err := fs.ReadDir(fsys, ent.Name())
		if err != nil {
			return nil, err
		}
		langs := map[string]*Language{}
		for _, f := range files {
			if f.IsDir() || !isPackFile(f.Name()) {
				continue
			}
			body, err := readPackFile(fsys, path.Join(ent.Name(), f.Name()))
			if err != nil {
				return nil, err
			}
			lang, err := decodeLanguage(body)
			if errors.Is(err, errSkipLegacyPack) {
				continue
			}
			if err != nil {
				return nil, err
			}
			langs[lang.Code] = lang
		}
		if len(langs) > 0 {
			platforms[platform] = langs
		}
	}
	cat := NewCatalog(platforms)
	cat.logAndroidXMLDrops()
	return cat, nil
}

// AndroidXMLDrops is how many strings indexLanguage removed from an Android
// pack because stock Android could not store them as XML. A repaired pack
// is zero.
func (c *Catalog) AndroidXMLDrops(code string) int {
	if c == nil {
		return 0
	}
	return c.androidXMLDrops[packCode(code)]
}

// AndroidXMLFixVersion is the Android pack version that replaces a cached
// locale file. Zero means the pack has no cutover.
func (c *Catalog) AndroidXMLFixVersion(platform, code string) int32 {
	l, ok := c.lookup(platform, code)
	if !ok {
		return 0
	}
	return l.meta.AndroidXMLFixVersion
}

func (c *Catalog) logAndroidXMLDrops() {
	if c == nil {
		return
	}
	total := 0
	codes := make([]string, 0, len(c.androidXMLDrops))
	for code, n := range c.androidXMLDrops {
		total += n
		if n > 0 {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	for _, code := range codes {
		logx.Errorf("langpack: android %s dropped %d strings that stock Android cannot store as XML", code, c.androidXMLDrops[code])
	}
	logx.Infof("langpack: android xml guard dropped %d strings", total)
}

func knownPlatformDir(name string) bool {
	switch name {
	case "ios", "android", "tdesktop", "macos":
		return true
	default:
		return false
	}
}

func isPackFile(name string) bool {
	return strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.gz")
}

func readPackFile(fsys fs.FS, name string) ([]byte, error) {
	body, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(name, ".gz") {
		return body, nil
	}
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func decodeLanguage(body []byte) (*Language, error) {
	var raw filePack
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	// Skip before CanonicalLang rewrites the code. Otherwise classic-zh-cn
	// would be stored as zh-hans and could replace the official pack.
	if _, ok := legacyLang(raw.LangCode); ok {
		return nil, errSkipLegacyPack
	}
	lang := &Language{
		Code:                 raw.LangCode,
		Name:                 raw.Name,
		NativeName:           raw.NativeName,
		PluralCode:           raw.PluralCode,
		Official:             raw.Official,
		RTL:                  raw.RTL,
		Beta:                 raw.Beta,
		BaseLangCode:         raw.BaseLangCode,
		TranslationsURL:      brand.Rewrite(raw.TranslationsURL),
		Version:              raw.Version,
		AndroidXMLFixVersion: raw.AndroidXMLFixVersion,
	}
	if canon, ok := CanonicalLang(lang.Code); ok {
		lang.Code = canon
	}
	for _, s := range raw.Strings {
		e := Entry{Key: s.Key, Value: brand.Rewrite(s.Value), Version: s.Version, Plural: rewritePlural(s.plural())}
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
		} else {
			defaultCat, defaultErr = Load(langpackdata.FS)
		}
		if defaultCat != nil {
			go defaultCat.prewarm()
		}
	})
	return defaultCat
}

// prewarm builds the packs the login screen and the first sync ask for,
// so that request is not the one that walks every string.
func (c *Catalog) prewarm() {
	for _, platform := range []string{"android", "ios", "tdesktop", "macos"} {
		_, _ = c.Languages(platform)
		for _, code := range []string{"en", "zh-hans", "zh-hant"} {
			_, _ = c.Difference(platform, code, 0)
		}
	}
}

// DefaultError is the error from loading DefaultCatalog, if any.
func DefaultError() error {
	DefaultCatalog()
	return defaultErr
}
