package richmsg

import (
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
)

func richFromPlain(text string, entities []*mtproto.MessageEntity, rtl bool) *mtproto.RichMessage {
	if len(entities) == 1 && coversAll(text, entities[0]) {
		switch entities[0].GetPredicateName() {
		case mtproto.Predicate_messageEntityBlockquote:
			plain := mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: text}).To_RichText()
			block := mtproto.MakeTLPageBlockBlockquote(&mtproto.PageBlock{Text: plain}).To_PageBlock()
			return richFromBlocks(rtl, []*mtproto.PageBlock{block})
		case mtproto.Predicate_messageEntityPre:
			plain := mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: text}).To_RichText()
			block := mtproto.MakeTLPageBlockPreformatted(&mtproto.PageBlock{
				Text:     plain,
				Language: entities[0].GetLanguage(),
			}).To_PageBlock()
			return richFromBlocks(rtl, []*mtproto.PageBlock{block})
		}
	}
	return paragraphRich(styleRich(text, entities), rtl)
}

func paragraphRich(text *mtproto.RichText, rtl bool) *mtproto.RichMessage {
	if text == nil {
		text = mtproto.MakeTLTextPlain(&mtproto.RichText{}).To_RichText()
	}
	block := mtproto.MakeTLPageBlockParagraph(&mtproto.PageBlock{Text: text}).To_PageBlock()
	return richFromBlocks(rtl, []*mtproto.PageBlock{block})
}

func coversAll(text string, ent *mtproto.MessageEntity) bool {
	if ent == nil || ent.GetOffset() != 0 || ent.GetLength() <= 0 {
		return false
	}
	return int(ent.GetLength()) == mtproto.UTF16Len(text)
}

func styleRich(text string, entities []*mtproto.MessageEntity) *mtproto.RichText {
	units := utf16.Encode([]rune(text))
	if len(units) == 0 {
		return mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: text}).To_RichText()
	}
	points := []int{0, len(units)}
	for _, ent := range entities {
		if ent == nil || ent.GetLength() <= 0 || ent.GetOffset() < 0 {
			continue
		}
		end := int(ent.GetOffset()) + int(ent.GetLength())
		if end > len(units) {
			continue
		}
		points = append(points, int(ent.GetOffset()), end)
	}
	points = uniqueInts(points)
	var parts []*mtproto.RichText
	for i := 0; i+1 < len(points); i++ {
		a, b := points[i], points[i+1]
		seg := string(utf16.Decode(units[a:b]))
		node := mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: seg}).To_RichText()
		var covering []*mtproto.MessageEntity
		for _, ent := range entities {
			if ent == nil || ent.GetLength() <= 0 {
				continue
			}
			if int(ent.GetOffset()) <= a && int(ent.GetOffset()+ent.GetLength()) >= b {
				covering = append(covering, ent)
			}
		}
		// Shortest span is the innermost wrapper.
		for i := 0; i < len(covering); i++ {
			for j := i + 1; j < len(covering); j++ {
				if covering[j].GetLength() < covering[i].GetLength() {
					covering[i], covering[j] = covering[j], covering[i]
				}
			}
		}
		for _, ent := range covering {
			node = wrapEntity(ent, node, seg)
		}
		parts = append(parts, node)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return mtproto.MakeTLTextConcat(&mtproto.RichText{Texts: parts}).To_RichText()
}

func wrapEntity(ent *mtproto.MessageEntity, inner *mtproto.RichText, segment string) *mtproto.RichText {
	switch ent.GetPredicateName() {
	case mtproto.Predicate_messageEntityBold:
		return mtproto.MakeTLTextBold(&mtproto.RichText{Text_RICHTEXT: inner}).To_RichText()
	case mtproto.Predicate_messageEntityItalic:
		return mtproto.MakeTLTextItalic(&mtproto.RichText{Text_RICHTEXT: inner}).To_RichText()
	case mtproto.Predicate_messageEntityUnderline:
		return mtproto.MakeTLTextUnderline(&mtproto.RichText{Text_RICHTEXT: inner}).To_RichText()
	case mtproto.Predicate_messageEntityStrike:
		return mtproto.MakeTLTextStrike(&mtproto.RichText{Text_RICHTEXT: inner}).To_RichText()
	case mtproto.Predicate_messageEntitySpoiler:
		return mtproto.MakeTLTextSpoiler(&mtproto.RichText{Text_RICHTEXT: inner}).To_RichText()
	case mtproto.Predicate_messageEntityCode, mtproto.Predicate_messageEntityPre:
		return mtproto.MakeTLTextFixed(&mtproto.RichText{Text_RICHTEXT: inner}).To_RichText()
	case mtproto.Predicate_messageEntityTextUrl:
		return mtproto.MakeTLTextUrl(&mtproto.RichText{Text_RICHTEXT: inner, Url: ent.GetUrl()}).To_RichText()
	case mtproto.Predicate_messageEntityUrl:
		return mtproto.MakeTLTextUrl(&mtproto.RichText{Text_RICHTEXT: inner, Url: segment}).To_RichText()
	case mtproto.Predicate_messageEntityEmail:
		return mtproto.MakeTLTextEmail(&mtproto.RichText{Text_RICHTEXT: inner, Email: segment}).To_RichText()
	case mtproto.Predicate_messageEntityPhone:
		return mtproto.MakeTLTextPhone(&mtproto.RichText{Text_RICHTEXT: inner, Phone: segment}).To_RichText()
	case mtproto.Predicate_messageEntityMention:
		return mtproto.MakeTLTextMention(&mtproto.RichText{Text_RICHTEXT: inner}).To_RichText()
	case mtproto.Predicate_messageEntityHashtag:
		return mtproto.MakeTLTextHashtag(&mtproto.RichText{Text_RICHTEXT: inner}).To_RichText()
	default:
		return inner
	}
}

func uniqueInts(points []int) []int {
	if len(points) < 2 {
		return points
	}
	for i := 1; i < len(points); i++ {
		for j := i; j > 0 && points[j] < points[j-1]; j-- {
			points[j], points[j-1] = points[j-1], points[j]
		}
	}
	out := points[:0]
	for _, p := range points {
		if len(out) == 0 || out[len(out)-1] != p {
			out = append(out, p)
		}
	}
	return out
}

func parseMarkdown(s string) (string, []*mtproto.MessageEntity) {
	var b strings.Builder
	var ents []*mtproto.MessageEntity
	parseMD(&b, &ents, s)
	return b.String(), ents
}

func parseMD(b *strings.Builder, ents *[]*mtproto.MessageEntity, s string) {
	for i := 0; i < len(s); {
		if s[i] == '`' {
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				inner := s[i+1 : i+1+end]
				start := mtproto.UTF16Len(b.String())
				b.WriteString(inner)
				if n := mtproto.UTF16Len(inner); n > 0 {
					*ents = append(*ents, &mtproto.MessageEntity{
						PredicateName: mtproto.Predicate_messageEntityCode,
						Offset:        int32(start),
						Length:        int32(n),
					})
				}
				i += end + 2
				continue
			}
		}
		if strings.HasPrefix(s[i:], "**") || strings.HasPrefix(s[i:], "__") {
			mark := s[i : i+2]
			if end := strings.Index(s[i+2:], mark); end >= 0 {
				start := mtproto.UTF16Len(b.String())
				parseMD(b, ents, s[i+2:i+2+end])
				if n := mtproto.UTF16Len(b.String()) - start; n > 0 {
					*ents = append(*ents, &mtproto.MessageEntity{
						PredicateName: mtproto.Predicate_messageEntityBold,
						Offset:        int32(start),
						Length:        int32(n),
					})
				}
				i += 2 + end + 2
				continue
			}
		}
		if s[i] == '[' {
			if label, url, next, ok := parseLink(s[i:]); ok {
				start := mtproto.UTF16Len(b.String())
				parseMD(b, ents, label)
				if n := mtproto.UTF16Len(b.String()) - start; n > 0 {
					*ents = append(*ents, &mtproto.MessageEntity{
						PredicateName: mtproto.Predicate_messageEntityTextUrl,
						Offset:        int32(start),
						Length:        int32(n),
						Url:           url,
					})
				}
				i += next
				continue
			}
		}
		if (s[i] == '*' || s[i] == '_') && (i+1 >= len(s) || s[i+1] != s[i]) {
			if end := strings.IndexByte(s[i+1:], s[i]); end > 0 {
				start := mtproto.UTF16Len(b.String())
				parseMD(b, ents, s[i+1:i+1+end])
				if n := mtproto.UTF16Len(b.String()) - start; n > 0 {
					*ents = append(*ents, &mtproto.MessageEntity{
						PredicateName: mtproto.Predicate_messageEntityItalic,
						Offset:        int32(start),
						Length:        int32(n),
					})
				}
				i += end + 2
				continue
			}
		}
		if s[i] == '#' && (i == 0 || s[i-1] == '\n') {
			j := i
			for j < len(s) && s[j] == '#' {
				j++
			}
			if j < len(s) && s[j] == ' ' {
				i = j + 1
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		i += size
	}
}

func parseLink(s string) (label, url string, n int, ok bool) {
	if len(s) < 4 || s[0] != '[' {
		return "", "", 0, false
	}
	closeLabel := strings.IndexByte(s, ']')
	if closeLabel < 1 || closeLabel+1 >= len(s) || s[closeLabel+1] != '(' {
		return "", "", 0, false
	}
	end := strings.IndexByte(s[closeLabel+2:], ')')
	if end < 0 {
		return "", "", 0, false
	}
	return s[1:closeLabel], s[closeLabel+2 : closeLabel+2+end], closeLabel + 2 + end + 1, true
}

var htmlToken = regexp.MustCompile(`(?i)<\s*(/?)\s*([a-z0-9]+)([^>]*)>`)
var htmlHref = regexp.MustCompile(`(?i)href\s*=\s*(?:"([^"]*)"|'([^']*)')`)

func parseHTML(s string) (string, []*mtproto.MessageEntity) {
	type frame struct {
		name string
		at   int
		url  string
	}
	var stack []frame
	var b strings.Builder
	var ents []*mtproto.MessageEntity
	rest := s
	for {
		loc := htmlToken.FindStringSubmatchIndex(rest)
		if loc == nil {
			b.WriteString(unescapeHTML(rest))
			break
		}
		b.WriteString(unescapeHTML(rest[:loc[0]]))
		closing := loc[2] >= 0 && loc[3] > loc[2] && rest[loc[2]:loc[3]] == "/"
		name := strings.ToLower(rest[loc[4]:loc[5]])
		attrs := ""
		if loc[6] >= 0 && loc[7] >= loc[6] {
			attrs = rest[loc[6]:loc[7]]
		}
		rest = rest[loc[1]:]
		switch name {
		case "br":
			b.WriteByte('\n')
			continue
		case "p", "div":
			if !closing && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
				b.WriteByte('\n')
			}
		}
		if !closing {
			stack = append(stack, frame{name: name, at: mtproto.UTF16Len(b.String()), url: htmlLink(attrs)})
			continue
		}
		for i := len(stack) - 1; i >= 0; i-- {
			if !sameHTML(stack[i].name, name) {
				continue
			}
			start := stack[i].at
			end := mtproto.UTF16Len(b.String())
			if ent := htmlEntity(stack[i].name, start, end-start, stack[i].url); ent != nil {
				ents = append(ents, ent)
			}
			stack = stack[:i]
			break
		}
	}
	return b.String(), ents
}

func unescapeHTML(s string) string {
	return strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&nbsp;", " ",
	).Replace(s)
}

func htmlLink(attrs string) string {
	m := htmlHref.FindStringSubmatch(attrs)
	if len(m) < 3 {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

func sameHTML(open, close string) bool {
	if open == close {
		return true
	}
	switch open + "/" + close {
	case "b/strong", "strong/b", "i/em", "em/i", "s/strike", "strike/s", "s/del", "del/s":
		return true
	default:
		return false
	}
}

func htmlEntity(name string, offset, length int, url string) *mtproto.MessageEntity {
	if length <= 0 {
		return nil
	}
	ent := &mtproto.MessageEntity{Offset: int32(offset), Length: int32(length)}
	switch name {
	case "b", "strong":
		ent.PredicateName = mtproto.Predicate_messageEntityBold
	case "i", "em":
		ent.PredicateName = mtproto.Predicate_messageEntityItalic
	case "u":
		ent.PredicateName = mtproto.Predicate_messageEntityUnderline
	case "s", "strike", "del":
		ent.PredicateName = mtproto.Predicate_messageEntityStrike
	case "code", "pre":
		ent.PredicateName = mtproto.Predicate_messageEntityCode
	case "a":
		ent.PredicateName = mtproto.Predicate_messageEntityTextUrl
		ent.Url = url
	default:
		return nil
	}
	return ent
}
