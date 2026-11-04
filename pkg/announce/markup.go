package announce

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Entity is one formatting span. Offset and Length are UTF-16 code units,
// the same unit Telegram uses for messageEntity.
type Entity struct {
	Type   string `json:"type"`
	Offset int32  `json:"offset"`
	Length int32  `json:"length"`
}

// Outbound is one private copy or the channel archive post.
type Outbound struct {
	From     int64
	Text     string
	Entities []Entity
}

// ParseMarkup turns **bold** markers into entities and removes the markers.
// Line breaks and bullet lines are kept. An unclosed marker stays as text.
func ParseMarkup(src string) (string, []Entity) {
	var b strings.Builder
	var ents []Entity
	for i := 0; i < len(src); {
		if strings.HasPrefix(src[i:], "**") {
			rest := src[i+2:]
			end := strings.Index(rest, "**")
			if end > 0 {
				inner := rest[:end]
				start := utf16Len(b.String())
				b.WriteString(inner)
				length := utf16Len(inner)
				if length > 0 {
					ents = append(ents, Entity{Type: "bold", Offset: start, Length: length})
				}
				i += 2 + end + 2
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(src[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteByte(src[i])
			i++
			continue
		}
		b.WriteRune(r)
		i += size
	}
	return b.String(), ents
}

func utf16Len(s string) int32 {
	return int32(len(utf16.Encode([]rune(s))))
}

func entitiesInRange(text string, ents []Entity) bool {
	n := utf16Len(text)
	for _, e := range ents {
		if e.Type != "bold" || e.Offset < 0 || e.Length < 1 || e.Offset+e.Length > n {
			return false
		}
	}
	return true
}
