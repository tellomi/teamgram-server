package catalog

import (
	"hash/fnv"
	"sort"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
)

// words returns this sticker's keywords for one canonical language.
// Untagged Keywords are English, except a word that contains Han (or kana
// or hangul), which is simplified Chinese. Explicit LangKeywords win for
// that language, including a Latin word tagged zh-hans.
func (item *Item) words(canon string) []string {
	if item == nil || canon == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(w string) {
		w = strings.TrimSpace(w)
		if w == "" || seen[w] {
			return
		}
		seen[w] = true
		out = append(out, w)
	}
	if canon == "en" {
		for _, w := range item.Keywords {
			if !containsCJK(w) {
				add(w)
			}
		}
	} else if canon == "zh-hans" {
		for _, w := range item.Keywords {
			if containsCJK(w) {
				add(w)
			}
		}
	}
	for _, w := range item.LangKeywords[canon] {
		add(w)
	}
	return out
}

func (item *Item) keywordHit(q string, langs []string) bool {
	q = foldText(q)
	if q == "" || item == nil {
		return false
	}
	hit := func(words []string) bool {
		for _, w := range words {
			if strings.Contains(foldText(w), q) {
				return true
			}
		}
		return false
	}
	if len(langs) == 0 {
		if hit(item.Keywords) {
			return true
		}
		for lang := range item.LangKeywords {
			if hit(item.words(lang)) {
				return true
			}
		}
		return false
	}
	for _, lang := range langs {
		if hit(item.words(lang)) {
			return true
		}
	}
	return false
}

// searchLangs is the keyword languages for one messages.searchStickers call.
// A nil result means every language. A Han query also searches zh-hans and
// zh-hant, because Android sends only the first IME language and an English
// keyboard can still type Chinese.
func searchLangs(requested []string, q string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(c string) {
		if c == "" || seen[c] {
			return
		}
		seen[c] = true
		out = append(out, c)
	}
	for _, code := range requested {
		add(canonicalEmojiLang(code))
	}
	if containsCJK(q) {
		add("zh-hans")
		add("zh-hant")
	}
	if len(out) == 0 {
		return nil
	}
	// English stays searchable when the IME language is Chinese. A Han query
	// does not substring-match the Latin keyword list.
	add("en")
	return out
}

type kwPair struct {
	word string
	emo  string
}

func collectKeywords(packs []*Pack, canon string, filter bool) []kwPair {
	type acc struct {
		emos []string
		seen map[string]bool
	}
	grouped := map[string]*acc{}
	for _, p := range packs {
		if p == nil || p.system() {
			continue
		}
		for _, item := range p.Items {
			if item == nil || item.Emoticon == "" {
				continue
			}
			if filter && emojiblock.Blocked(item.Emoticon) {
				continue
			}
			for _, word := range item.words(canon) {
				key := foldText(word)
				if key == "" {
					continue
				}
				a := grouped[key]
				if a == nil {
					a = &acc{seen: map[string]bool{}}
					grouped[key] = a
				}
				if !a.seen[item.Emoticon] {
					a.seen[item.Emoticon] = true
					a.emos = append(a.emos, item.Emoticon)
				}
			}
		}
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var pairs []kwPair
	for _, key := range keys {
		emos := grouped[key].emos
		sort.Strings(emos)
		for _, emo := range emos {
			pairs = append(pairs, kwPair{word: key, emo: emo})
		}
	}
	return pairs
}

func keywordSum(pairs []kwPair) uint64 {
	h := fnv.New64a()
	for _, p := range pairs {
		_, _ = h.Write([]byte(p.word))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(p.emo))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}

var (
	enBaselineOnce sync.Once
	enBaselineSum  uint64
)

// enBaselineSumOf is the unfiltered English keyword sum of the built-in packs.
// The default blocklist does not remove any seeded emoji, so a catalog that
// still has only those English words keeps messages.getEmojiKeywords version 1.
func enBaseline() uint64 {
	enBaselineOnce.Do(func() {
		enBaselineSum = keywordSum(collectKeywords(builtinPacks(), "en", false))
	})
	return enBaselineSum
}

func (s *Service) keywordVersion(canon string) int32 {
	sum := keywordSum(collectKeywords(s.packs, canon, true))
	base := emojiKeywordVersion()
	if canon == "en" && base == 1 && sum == enBaseline() {
		return 1
	}
	v := int32(sum & 0x7fffffff)
	if base != 1 {
		v ^= base
		if v < 0 {
			v = -v
		}
	}
	if v <= 1 {
		return 2
	}
	return v
}

func (s *Service) availableLangs() []string {
	has := map[string]bool{}
	for _, p := range s.packs {
		if p == nil || p.system() {
			continue
		}
		for _, item := range p.Items {
			if item == nil || emojiblock.Blocked(item.Emoticon) {
				continue
			}
			if len(item.words("en")) > 0 {
				has["en"] = true
			}
			if len(item.words("zh-hans")) > 0 {
				has["zh-hans"] = true
			}
			if len(item.words("zh-hant")) > 0 {
				has["zh-hant"] = true
			}
			for lang := range item.LangKeywords {
				if len(item.words(lang)) > 0 {
					has[lang] = true
				}
			}
		}
	}
	order := []string{"en", "zh-hans", "zh-hant"}
	var extra []string
	for lang := range has {
		switch lang {
		case "en", "zh-hans", "zh-hant":
		default:
			extra = append(extra, lang)
		}
	}
	sort.Strings(extra)
	var out []string
	for _, lang := range append(order, extra...) {
		if has[lang] {
			out = append(out, lang)
		}
	}
	return out
}

func emojiLang(code string) *mtproto.EmojiLanguage {
	return mtproto.MakeTLEmojiLanguage(&mtproto.EmojiLanguage{LangCode: code}).To_EmojiLanguage()
}

// Languages answers messages.getEmojiKeywordsLanguages.
// Desktop is the client that calls it: it sends the keyboard and app
// languages, then fetches keywords only for the codes in the reply.
// The returned lang_code is the code the client sent, not a rewritten
// canonical form, because desktop drops the list when the two differ
// and iOS difference updates do the same.
// An empty request returns every language that currently has keywords.
func (s *Service) Languages(codes []string) *mtproto.Vector_EmojiLanguage {
	s.mu.Lock()
	defer s.mu.Unlock()
	have := map[string]bool{}
	for _, lang := range s.availableLangs() {
		have[lang] = true
	}
	var datas []*mtproto.EmojiLanguage
	if len(codes) == 0 {
		for _, lang := range s.availableLangs() {
			datas = append(datas, emojiLang(lang))
		}
	} else {
		seen := map[string]bool{}
		for _, code := range codes {
			trimmed := strings.TrimSpace(code)
			if trimmed == "" || seen[trimmed] {
				continue
			}
			if !have[canonicalEmojiLang(trimmed)] {
				continue
			}
			seen[trimmed] = true
			datas = append(datas, emojiLang(trimmed))
		}
	}
	if datas == nil {
		datas = []*mtproto.EmojiLanguage{}
	}
	return &mtproto.Vector_EmojiLanguage{Datas: datas}
}

// Keywords answers messages.getEmojiKeywords and getEmojiKeywordsDifference.
// lang_code in the reply is the requested code, unchanged, so a client that
// asked for zh-CN, zh-hans, or zh stores the Chinese list under that key.
func (s *Service) Keywords(lang string, from int32) *mtproto.EmojiKeywordsDifference {
	s.mu.Lock()
	defer s.mu.Unlock()
	requested := strings.TrimSpace(lang)
	canon := canonicalEmojiLang(requested)
	version := s.keywordVersion(canon)
	diff := &mtproto.EmojiKeywordsDifference{
		LangCode:    requested,
		FromVersion: from,
		Version:     version,
		Keywords:    []*mtproto.EmojiKeyword{},
	}
	if canon == "" || from >= version {
		return mtproto.MakeTLEmojiKeywordsDifference(diff).To_EmojiKeywordsDifference()
	}
	pairs := collectKeywords(s.packs, canon, true)
	var cur string
	var emos []string
	flush := func() {
		if cur == "" || len(emos) == 0 {
			return
		}
		diff.Keywords = append(diff.Keywords, mtproto.MakeTLEmojiKeyword(&mtproto.EmojiKeyword{
			Keyword:   cur,
			Emoticons: append([]string(nil), emos...),
		}).To_EmojiKeyword())
	}
	for _, p := range pairs {
		if p.word != cur {
			flush()
			cur = p.word
			emos = emos[:0]
		}
		emos = append(emos, p.emo)
	}
	flush()
	return mtproto.MakeTLEmojiKeywordsDifference(diff).To_EmojiKeywordsDifference()
}
