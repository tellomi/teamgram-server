// Package lexiconsrc checks the pinned cn politics word lists.
// Classify repeats the D-25 admission rules. PromotedTerms is the overlap
// slice the username engine loads on top of policy-2026110602.
// SupplementTerms is a reviewed allowlist of exact lines from the
// unvendored GFW supplement file. The file itself is not stored here.
//
// The vendored files, licenses, and the refresh command are described in
// SOURCE.toml. Vocabulary/零时-Tencent.txt is not stored here. generated/
// keeps only the overlap keys needed to repeat the D-25 admission rules.
package lexiconsrc

import (
	"bufio"
	"crypto/sha1"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

//go:embed konsheng fwwdn editorial generated licenses
var pack embed.FS

const (
	// Counts produced by update.py from the pins in SOURCE.toml.
	AdmitCount        = 200
	PinyinRuleCount   = 191
	DeferTencentCount = 804
	DeferBothCount    = 7
	DropCount         = 15
	DropListRows      = 62
	OverlapKeyCount   = 804
	PolicyRuleCount   = AdmitCount + PinyinRuleCount
)

// BlobPins are git blob ids of the vendored upstream bytes.
var BlobPins = map[string]string{
	"konsheng/政治类型.txt":             "04f9c44077ecc86f217778a2399afd7f81d56d1f",
	"konsheng/反动词库.txt":             "2e5e59a62b1e797a44e171e88623d71f7794a68a",
	"konsheng/贪腐词库.txt":             "c21cfdf3a74ecb761cb67a22477f4158fa97757d",
	"fwwdn/政治类.txt":                 "1a41ef011fce6f7e181742af86f9ba26a40a9a2a",
	"licenses/konsheng-LICENSE.txt": "497bd06ea1c33bbe0fcf3abcc6d7d70dd3493207",
	"licenses/fwwdn-LICENSE.txt":    "58ff8cbaa04aff6100f650ea8aff9f50d80933ca",
}

const trailingPunct = "。．.，,、;；"

var (
	usernameFields = []string{"username", "slug", "bot_username"}
	displayFields  = []string{
		"display_name", "group_name", "group_description", "channel_name",
		"org_name", "bio", "search",
	}
	fold = cases.Fold()
)

// Report is the D-25 classification of the vendored category files.
type Report struct {
	Admit        []string
	Pinyin       []string
	DeferBoth    []string
	DeferTencent int
	Drop         int
	PinyinOf     map[string]string
}

// TermSpec is the match list and field list a published rule must have.
type TermSpec struct {
	Matches []string
	Fields  []string
	Pinyin  bool
}

// Classify reads the vendored files and applies the admission rules.
func Classify() (Report, error) {
	var rep Report
	if err := CheckBlobs(); err != nil {
		return rep, err
	}
	forms := map[string]map[string]struct{}{}
	sources := map[string]map[string]struct{}{}
	addFile := func(path, upstream string, stripComma bool) error {
		text, err := readText(path)
		if err != nil {
			return err
		}
		for _, word := range parseLines(text, stripComma) {
			key := dedupKey(word)
			if forms[key] == nil {
				forms[key] = map[string]struct{}{}
				sources[key] = map[string]struct{}{}
			}
			forms[key][word] = struct{}{}
			sources[key][upstream] = struct{}{}
		}
		return nil
	}
	for _, path := range []string{"konsheng/政治类型.txt", "konsheng/反动词库.txt", "konsheng/贪腐词库.txt"} {
		if err := addFile(path, "konsheng", false); err != nil {
			return rep, err
		}
	}
	if err := addFile("fwwdn/政治类.txt", "fwwdn", true); err != nil {
		return rep, err
	}
	drops, err := loadDrops()
	if err != nil {
		return rep, err
	}
	if len(drops) != DropListRows {
		return rep, fmt.Errorf("lexiconsrc: drop list has %d rows", len(drops))
	}
	for word := range drops {
		if _, ok := forms[dedupKey(word)]; !ok {
			return rep, fmt.Errorf("lexiconsrc: drop %q is not in the category files", word)
		}
	}
	overlap, err := loadOverlap()
	if err != nil {
		return rep, err
	}
	if len(overlap) != OverlapKeyCount {
		return rep, fmt.Errorf("lexiconsrc: overlap keys %d", len(overlap))
	}
	blocked, err := loadBlock()
	if err != nil {
		return rep, err
	}
	pinyinOf, err := loadPinyin()
	if err != nil {
		return rep, err
	}

	type row struct {
		word, source, decision string
	}
	var rows []row
	for key, words := range forms {
		word := chooseDisplay(words)
		source := sourceOf(sources[key])
		var decision string
		switch {
		case overlap[key]:
			decision = "defer-tencent"
		case drops[word] != "":
			decision = "drop"
		case source == "both":
			decision = "defer-both"
		default:
			decision = "admit"
		}
		rows = append(rows, row{word, source, decision})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].word < rows[j].word })

	rep.PinyinOf = map[string]string{}
	seen := map[string]struct{}{}
	for _, row := range rows {
		switch row.decision {
		case "defer-tencent":
			rep.DeferTencent++
		case "drop":
			rep.Drop++
		case "defer-both":
			rep.DeferBoth = append(rep.DeferBoth, row.word)
		case "admit":
			rep.Admit = append(rep.Admit, row.word)
			seen[row.word] = struct{}{}
			py, ok := pinyinOf[row.word]
			if !ok {
				return rep, fmt.Errorf("lexiconsrc: admit %q has no pinyin row", row.word)
			}
			rep.PinyinOf[row.word] = py
		default:
			return rep, fmt.Errorf("lexiconsrc: bad decision %s", row.decision)
		}
	}
	if len(pinyinOf) != len(rep.Admit) {
		return rep, fmt.Errorf("lexiconsrc: pinyin rows %d admit %d", len(pinyinOf), len(rep.Admit))
	}
	for _, word := range rep.Admit {
		py := rep.PinyinOf[word]
		if py == "" || py == dedupKey(word) || blocked[py] {
			continue
		}
		if _, ok := seen[py]; ok || !usernameSafe(py) {
			continue
		}
		seen[py] = struct{}{}
		rep.Pinyin = append(rep.Pinyin, py)
	}
	sort.Strings(rep.Pinyin)
	if rep.DeferTencent != DeferTencentCount || rep.Drop != DropCount || len(rep.DeferBoth) != DeferBothCount {
		return rep, fmt.Errorf("lexiconsrc: tencent %d drop %d both %d", rep.DeferTencent, rep.Drop, len(rep.DeferBoth))
	}
	if len(rep.Admit) != AdmitCount || len(rep.Pinyin) != PinyinRuleCount {
		return rep, fmt.Errorf("lexiconsrc: admit %d pinyin %d", len(rep.Admit), len(rep.Pinyin))
	}
	return rep, nil
}

// SpecFor reports the matches and fields the tellomi build attaches to term.
// pinyinRule is true when term is a generated reading rather than a source line.
func SpecFor(term string, pinyinRule bool) TermSpec {
	spec := TermSpec{Pinyin: pinyinRule}
	if pinyinRule {
		spec.Matches = []string{"NORMALIZED_EXACT"}
		if utf8.RuneCountInString(term) >= 8 {
			spec.Matches = append(spec.Matches, "PREFIX", "SUFFIX")
		}
		if utf8.RuneCountInString(term) >= 4 {
			spec.Matches = append(spec.Matches, "CONFUSABLE")
		}
		spec.Fields = append(append([]string{}, usernameFields...), "display_name")
		return spec
	}
	switch {
	case utf8.RuneCountInString(term) <= 2:
		spec.Matches = []string{"NORMALIZED_EXACT"}
	case cjkCount(term) >= 2:
		spec.Matches = []string{"NORMALIZED_EXACT", "CONTAINS"}
	case usernameSafe(term) && utf8.RuneCountInString(term) >= 5:
		spec.Matches = []string{"NORMALIZED_EXACT", "PREFIX", "SUFFIX"}
	default:
		spec.Matches = []string{"NORMALIZED_EXACT"}
	}
	if usernameSafe(term) {
		spec.Fields = append(append([]string{}, usernameFields...), displayFields...)
		return spec
	}
	spec.Fields = append([]string{}, displayFields...)
	return spec
}

// CheckBlobs compares vendored bytes with the git blob ids in SOURCE.toml.
func CheckBlobs() error {
	for path, want := range BlobPins {
		data, err := pack.ReadFile(path)
		if err != nil {
			return err
		}
		if got := gitBlob(data); got != want {
			return fmt.Errorf("lexiconsrc: %s blob %s", path, got)
		}
	}
	return nil
}

// CoverageJSON is the summary update.py writes.
func CoverageJSON() (map[string]any, error) {
	data, err := pack.ReadFile("generated/coverage.json")
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func gitBlob(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func readText(path string) (string, error) {
	data, err := pack.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(string(data), "\uFEFF"), nil
}

func parseLines(text string, stripComma bool) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		word := strings.TrimSpace(sc.Text())
		if stripComma {
			word = strings.TrimSpace(strings.TrimRight(word, ","))
		}
		word = strings.TrimSpace(strings.TrimRight(word, trailingPunct))
		if word == "" || strings.HasPrefix(word, "#") {
			continue
		}
		out = append(out, word)
	}
	return out
}

func dedupKey(word string) string {
	return fold.String(norm.NFKC.String(word))
}

func chooseDisplay(forms map[string]struct{}) string {
	list := make([]string, 0, len(forms))
	for word := range forms {
		list = append(list, word)
	}
	sort.Slice(list, func(i, j int) bool {
		ai := list[i] != dedupKey(list[i])
		aj := list[j] != dedupKey(list[j])
		if ai != aj {
			return !ai
		}
		return list[i] < list[j]
	})
	return list[0]
}

func sourceOf(srcs map[string]struct{}) string {
	_, k := srcs["konsheng"]
	_, f := srcs["fwwdn"]
	switch {
	case k && f:
		return "both"
	case k:
		return "konsheng"
	default:
		return "fwwdn"
	}
}

func loadDrops() (map[string]string, error) {
	text, err := readText("editorial/politics-drops.tsv")
	if err != nil {
		return nil, err
	}
	drops := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		word, reason, ok := strings.Cut(raw, "\t")
		if !ok || word == "" || reason == "" {
			return nil, fmt.Errorf("lexiconsrc: bad drop line %q", raw)
		}
		if word == "word" && reason == "reason" {
			continue
		}
		drops[word] = reason
	}
	return drops, nil
}

func loadOverlap() (map[string]bool, error) {
	text, err := readText("generated/tencent-overlap-keys.txt")
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		out[raw] = true
	}
	return out, nil
}

func loadBlock() (map[string]bool, error) {
	text, err := readText("editorial/pinyin-homophone-block.txt")
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		raw, _, _ := strings.Cut(line, "#")
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		out[strings.ToLower(strings.Fields(raw)[0])] = true
	}
	return out, nil
}

func loadPinyin() (map[string]string, error) {
	text, err := readText("generated/admit-pinyin.tsv")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		word, py, ok := strings.Cut(raw, "\t")
		if !ok || word == "" || py == "" {
			return nil, fmt.Errorf("lexiconsrc: bad pinyin row %q", raw)
		}
		if _, dup := out[word]; dup {
			return nil, fmt.Errorf("lexiconsrc: duplicate pinyin row %s", word)
		}
		out[word] = py
	}
	return out, nil
}

func usernameSafe(term string) bool {
	if term == "" || utf8.RuneCountInString(term) < 3 || utf8.RuneCountInString(term) > 32 {
		return false
	}
	for i, r := range term {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' || r == '_':
			if i == 0 && r != '_' {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func cjkCount(term string) int {
	n := 0
	for _, r := range term {
		if r >= 0x3400 && r <= 0x9FFF {
			n++
		}
	}
	return n
}

// RuntimeTerm is one MIT-file line the username engine loads in addition
// to policy-2026110602. Files are vendored konsheng paths. fwwdn is not a
// second license: its 政治类.txt is the same text as 政治类型.txt.
type RuntimeTerm struct {
	Term  string
	Files []string
}

// PromotedTerms is the defer-tencent slice of the vendored konsheng category
// files after the recorded drop list and editorial/runtime-defer.tsv.
// D-25 still reports those lines as defer-tencent. The Tencent file is not
// read and not vendored. source=both names that are not in the Tencent
// overlap stay deferred. Porn, violence, and ads files are not inputs.
// editorial/runtime-exact.tsv does not remove a term; it only suppresses
// CONTAINS for that term. SupplementTerms is separate.
func PromotedTerms() ([]RuntimeTerm, error) {
	if err := CheckBlobs(); err != nil {
		return nil, err
	}
	forms := map[string]map[string]struct{}{}
	sources := map[string]map[string]struct{}{}
	files := map[string]map[string]struct{}{}
	addFile := func(path, upstream string, stripComma bool) error {
		text, err := readText(path)
		if err != nil {
			return err
		}
		for _, word := range parseLines(text, stripComma) {
			key := dedupKey(word)
			if forms[key] == nil {
				forms[key] = map[string]struct{}{}
				sources[key] = map[string]struct{}{}
				files[key] = map[string]struct{}{}
			}
			forms[key][word] = struct{}{}
			sources[key][upstream] = struct{}{}
			if upstream == "konsheng" {
				files[key][path] = struct{}{}
			}
		}
		return nil
	}
	for _, path := range []string{"konsheng/政治类型.txt", "konsheng/反动词库.txt", "konsheng/贪腐词库.txt"} {
		if err := addFile(path, "konsheng", false); err != nil {
			return nil, err
		}
	}
	if err := addFile("fwwdn/政治类.txt", "fwwdn", true); err != nil {
		return nil, err
	}
	drops, err := loadDrops()
	if err != nil {
		return nil, err
	}
	overlap, err := loadOverlap()
	if err != nil {
		return nil, err
	}
	extra, err := loadRuntimeDefer()
	if err != nil {
		return nil, err
	}
	dropKeys := map[string]struct{}{}
	for word := range drops {
		dropKeys[dedupKey(word)] = struct{}{}
	}
	var out []RuntimeTerm
	seenExtra := map[string]struct{}{}
	for key, words := range forms {
		if !overlap[key] {
			continue
		}
		word := chooseDisplay(words)
		if _, dropped := drops[word]; dropped {
			continue
		}
		if _, dropped := dropKeys[key]; dropped {
			continue
		}
		if _, skip := extra[key]; skip {
			seenExtra[key] = struct{}{}
			continue
		}
		if len(files[key]) == 0 {
			return nil, fmt.Errorf("lexiconsrc: promoted %s has no konsheng file", word)
		}
		list := make([]string, 0, len(files[key]))
		for path := range files[key] {
			list = append(list, path)
		}
		sort.Strings(list)
		out = append(out, RuntimeTerm{Term: word, Files: list})
	}
	if len(seenExtra) != len(extra) {
		for key := range extra {
			if _, ok := seenExtra[key]; !ok {
				return nil, fmt.Errorf("lexiconsrc: runtime-defer %s is not a Tencent-overlap category line", key)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Term < out[j].Term })
	return out, nil
}

// RuntimePinyin reads generated/runtime-pinyin.tsv. term is the source line,
// pinyin is the pypinyin 0.55.0 reading that becomes its own latin rule.
func RuntimePinyin() (map[string]string, error) {
	text, err := readText("generated/runtime-pinyin.tsv")
	if err != nil {
		return nil, err
	}
	blocked, err := loadBlock()
	if err != nil {
		return nil, err
	}
	promoted, err := PromotedTerms()
	if err != nil {
		return nil, err
	}
	supplement, err := SupplementTerms()
	if err != nil {
		return nil, err
	}
	allowed := map[string]RuntimeTerm{}
	for _, term := range promoted {
		allowed[term.Term] = term
	}
	for _, term := range supplement {
		if _, ok := allowed[term.Term]; ok {
			return nil, fmt.Errorf("lexiconsrc: supplement %s duplicates a promoted term", term.Term)
		}
		allowed[term.Term] = term
	}
	out := map[string]string{}
	seenPy := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		word, py, ok := strings.Cut(raw, "\t")
		if !ok || word == "" || py == "" {
			return nil, fmt.Errorf("lexiconsrc: bad runtime pinyin row %q", raw)
		}
		if _, ok := allowed[word]; !ok {
			return nil, fmt.Errorf("lexiconsrc: runtime pinyin %s is not a runtime term", word)
		}
		if py == dedupKey(word) || !usernameSafe(py) || utf8.RuneCountInString(py) < 8 || blocked[py] {
			return nil, fmt.Errorf("lexiconsrc: runtime pinyin %s -> %s rejected", word, py)
		}
		if prev, ok := seenPy[py]; ok {
			return nil, fmt.Errorf("lexiconsrc: pinyin %s from both %s and %s", py, prev, word)
		}
		seenPy[py] = word
		if _, dup := out[word]; dup {
			return nil, fmt.Errorf("lexiconsrc: duplicate runtime pinyin row %s", word)
		}
		out[word] = py
	}
	return out, nil
}

func loadRuntimeDefer() (map[string]string, error) {
	text, err := readText("editorial/runtime-defer.tsv")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		word, reason, ok := strings.Cut(raw, "\t")
		if !ok || word == "" || reason == "" {
			return nil, fmt.Errorf("lexiconsrc: bad runtime-defer line %q", raw)
		}
		if word == "word" && reason == "reason" {
			continue
		}
		key := dedupKey(word)
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("lexiconsrc: duplicate runtime-defer %s", word)
		}
		out[key] = reason
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("lexiconsrc: runtime-defer is empty")
	}
	return out, nil
}

// FragmentExact reports three-character terms whose CONTAINS match hits a
// longer ordinary name. 四事件 hits 五四事件, 上海帮 hits 上海帮忙, and
// 上中央 hits 上中央电视台. Names such as 辛灏年 are not this shape.
// Two-character terms are already exact. Terms of four or more characters
// stay on CONTAINS.
func FragmentExact(term string) bool {
	if utf8.RuneCountInString(term) != 3 {
		return false
	}
	return strings.HasSuffix(term, "事件") ||
		strings.HasSuffix(term, "中央") ||
		strings.HasSuffix(term, "帮") ||
		strings.HasSuffix(term, "门") ||
		strings.HasSuffix(term, "社") ||
		strings.HasSuffix(term, "论")
}

// RuntimeExact lists promoted terms that stay loaded but must not use
// CONTAINS. Two-character terms are already exact and are rejected here.
// FragmentExact terms are separate: they are not repeated in this file.
func RuntimeExact() (map[string]string, error) {
	rows, err := loadWordReasons("editorial/runtime-exact.tsv", "runtime-exact")
	if err != nil {
		return nil, err
	}
	promoted, err := PromotedTerms()
	if err != nil {
		return nil, err
	}
	allowed := map[string]struct{}{}
	for _, term := range promoted {
		allowed[term.Term] = struct{}{}
	}
	out := map[string]string{}
	for _, row := range rows {
		word, reason := row[0], row[1]
		if _, ok := allowed[word]; !ok {
			return nil, fmt.Errorf("lexiconsrc: runtime-exact %s is not a promoted term", word)
		}
		contains := false
		for _, match := range SpecFor(word, false).Matches {
			if match == "CONTAINS" {
				contains = true
			}
		}
		if !contains {
			return nil, fmt.Errorf("lexiconsrc: runtime-exact %s is already exact", word)
		}
		out[word] = reason
	}
	return out, nil
}

// SupplementTerms reads generated/gfw-selected.tsv. update.py --fetch
// rebuilds that file from the pinned GFW blob: a line is kept only when it
// contains a stem, avoids the skip markers, and is not already covered by
// a loaded CONTAINS rule. The GFW file itself is not vendored.
func SupplementTerms() ([]RuntimeTerm, error) {
	if err := CheckBlobs(); err != nil {
		return nil, err
	}
	text, err := readText("generated/gfw-selected.tsv")
	if err != nil {
		return nil, err
	}
	words := parseLines(text, false)
	if len(words) == 0 {
		return nil, fmt.Errorf("lexiconsrc: gfw-selected is empty")
	}
	stems, err := loadWordList("editorial/gfw-stems.txt")
	if err != nil {
		return nil, err
	}
	markers, err := loadWordList("editorial/gfw-skip-markers.txt")
	if err != nil {
		return nil, err
	}
	keys, err := categoryKeys()
	if err != nil {
		return nil, err
	}
	deferred, err := loadRuntimeDefer()
	if err != nil {
		return nil, err
	}
	banned := map[string]struct{}{
		"天安门": {}, "新疆": {}, "平反": {}, "静坐": {}, "法轮": {},
		"五四事件": {}, "黄色图片": {}, "仿真枪": {}, "大屠杀": {},
	}
	var out []RuntimeTerm
	seen := map[string]struct{}{}
	for _, word := range words {
		key := dedupKey(word)
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("lexiconsrc: duplicate gfw selected %s", word)
		}
		seen[key] = struct{}{}
		if _, ok := banned[word]; ok {
			return nil, fmt.Errorf("lexiconsrc: gfw selected must not include %s", word)
		}
		if utf8.RuneCountInString(word) < 3 || cjkCount(word) < 2 {
			return nil, fmt.Errorf("lexiconsrc: gfw selected %s fails the length gate", word)
		}
		if !containsAny(word, stems) {
			return nil, fmt.Errorf("lexiconsrc: gfw selected %s has no stem", word)
		}
		if marker := firstContained(word, markers); marker != "" {
			return nil, fmt.Errorf("lexiconsrc: gfw selected %s contains marker %s", word, marker)
		}
		if _, ok := keys[key]; ok {
			return nil, fmt.Errorf("lexiconsrc: gfw selected %s is already a category-file line", word)
		}
		if _, ok := deferred[key]; ok {
			return nil, fmt.Errorf("lexiconsrc: gfw selected %s is runtime-deferred", word)
		}
		out = append(out, RuntimeTerm{Term: word, Files: []string{"generated/gfw-selected.tsv"}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Term < out[j].Term })
	return out, nil
}

func loadWordList(path string) ([]string, error) {
	text, err := readText(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		out = append(out, raw)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("lexiconsrc: %s is empty", path)
	}
	return out, nil
}

func containsAny(word string, parts []string) bool {
	for _, part := range parts {
		if part != "" && strings.Contains(word, part) {
			return true
		}
	}
	return false
}

func firstContained(word string, parts []string) string {
	for _, part := range parts {
		if part != "" && strings.Contains(word, part) {
			return part
		}
	}
	return ""
}

func categoryKeys() (map[string]struct{}, error) {
	out := map[string]struct{}{}
	for _, path := range []string{"konsheng/政治类型.txt", "konsheng/反动词库.txt", "konsheng/贪腐词库.txt"} {
		text, err := readText(path)
		if err != nil {
			return nil, err
		}
		for _, word := range parseLines(text, false) {
			out[dedupKey(word)] = struct{}{}
		}
	}
	return out, nil
}

func loadWordReasons(path, label string) ([][2]string, error) {
	text, err := readText(path)
	if err != nil {
		return nil, err
	}
	var rows [][2]string
	seen := map[string]struct{}{}
	for _, line := range strings.Split(text, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		word, reason, ok := strings.Cut(raw, "\t")
		if !ok || word == "" || reason == "" {
			return nil, fmt.Errorf("lexiconsrc: bad %s line %q", label, raw)
		}
		if word == "word" && reason == "reason" {
			continue
		}
		key := dedupKey(word)
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("lexiconsrc: duplicate %s %s", label, word)
		}
		seen[key] = struct{}{}
		rows = append(rows, [2]string{word, reason})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("lexiconsrc: %s is empty", label)
	}
	return rows, nil
}
