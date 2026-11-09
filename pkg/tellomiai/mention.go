package tellomiai

import (
	"sort"
	"strings"
	"unicode/utf16"
)

const (
	// Username is the @tellomi mention, without the @.
	Username = "tellomi"
	// MentionUsername is a messageEntityMention of @tellomi.
	MentionUsername = "mention"
	// MentionName is a messageEntityMentionName (or input mention name) of 777001.
	MentionName = "name"
)

// Mention is one entity on the triggering message. Offset and Length are
// UTF-16 code units, the same unit Telegram uses.
type Mention struct {
	Kind   string
	Offset int
	Length int
	UserID int64
}

// StripMention reports whether text mentions @tellomi and returns the text
// with those spans removed. plain is true only when the client sent no
// entities at all; then a bare @tellomi token counts, because the server
// would store it as a username mention. When the client sent entities, only
// those entities count.
func StripMention(text string, ents []Mention, plain bool) (string, bool) {
	if len(ents) == 0 {
		if !plain {
			return strings.TrimSpace(text), false
		}
		return stripPlain(text)
	}
	units := utf16.Encode([]rune(text))
	type span struct{ start, end int }
	var cuts []span
	mentioned := false
	for _, e := range ents {
		if e.Length <= 0 || e.Offset < 0 {
			continue
		}
		end := e.Offset + e.Length
		if end > len(units) {
			continue
		}
		raw := string(utf16.Decode(units[e.Offset:end]))
		if !tellomiSpan(e, raw) {
			continue
		}
		cuts = append(cuts, span{e.Offset, end})
		mentioned = true
	}
	if !mentioned {
		return strings.TrimSpace(text), false
	}
	// Drop from the right so earlier offsets stay valid.
	sort.Slice(cuts, func(i, j int) bool { return cuts[i].start > cuts[j].start })
	for _, c := range cuts {
		if c.start < 0 || c.end > len(units) || c.start >= c.end {
			continue
		}
		units = append(units[:c.start], units[c.end:]...)
	}
	return tidySpaces(string(utf16.Decode(units))), true
}

func tellomiSpan(e Mention, raw string) bool {
	switch e.Kind {
	case MentionName:
		return e.UserID == AccountID
	case MentionUsername:
		if e.UserID == AccountID {
			return true
		}
		name := strings.TrimPrefix(raw, "@")
		return strings.EqualFold(name, Username)
	default:
		return false
	}
}

func stripPlain(text string) (string, bool) {
	rs := []rune(text)
	lower := []rune(strings.ToLower(text))
	needle := []rune("@" + Username)
	var out []rune
	mentioned := false
	for i := 0; i < len(rs); {
		if i+len(needle) <= len(lower) && string(lower[i:i+len(needle)]) == string(needle) && tokenBoundary(rs, i, len(needle)) {
			mentioned = true
			i += len(needle)
			continue
		}
		out = append(out, rs[i])
		i++
	}
	if !mentioned {
		return strings.TrimSpace(text), false
	}
	return tidySpaces(string(out)), true
}

func tokenBoundary(rs []rune, i, n int) bool {
	if i > 0 && isUserChar(rs[i-1]) {
		return false
	}
	end := i + n
	if end < len(rs) && isUserChar(rs[end]) {
		return false
	}
	return true
}

func isUserChar(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func tidySpaces(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if space {
				continue
			}
			space = true
			b.WriteRune(' ')
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// GroupPrompt is the only text a group turn sends to the model: the triggering
// message with the @tellomi mention removed, and, when this message is a
// reply, the single replied-to message. Nothing else from the chat is included.
func GroupPrompt(question, replied string) string {
	question = strings.TrimSpace(question)
	replied = strings.TrimSpace(replied)
	switch {
	case question == "" && replied == "":
		return ""
	case replied == "":
		return question
	case question == "":
		return replied
	default:
		return replied + "\n\n" + question
	}
}

// ChatKey identifies one basic group or supergroup for the per-chat cap.
func ChatKey(peerType int32, peerID int64) string {
	return itoa(int64(peerType)) + ":" + itoa(peerID)
}
