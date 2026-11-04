package push

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/teamgram-server/pkg/langpack"
)

// NormalizeLang maps one language tag onto a pack this server ships.
// zh, zh-CN and zh-Hans are simplified. zh-Hant, zh-TW and zh-HK are
// traditional. classic-zh-cn is zh-hans and taiwan is zh-hant. Every other
// official code (ja, ru, …) stays itself. An unknown tag is English.
func NormalizeLang(code string) string {
	if c, ok := langpack.CanonicalLang(code); ok {
		return c
	}
	return "en"
}

// PickLang is the language a push is rendered in.
//
// The app language wins when it is a specific pack (zh-hans, ja, …).
// English does not win over the phone language: the iOS notification
// extension opens a supplementary connection with lang_code "en" because it
// does not load the saved pack, and that initConnection overwrites
// auths.lang_code. The phone language (initConnection.system_lang_code) is
// still zh-Hans on a 简体中文 iPhone, and that is what the banner should use.
// A non-English app language still wins over the phone, so 简体中文 on an
// English phone stays simplified Chinese.
func PickLang(app, system string) string {
	if c, ok := specificLang(app); ok {
		return c
	}
	if c, ok := specificLang(system); ok {
		return c
	}
	if c, ok := langpack.CanonicalLang(app); ok {
		return c
	}
	if c, ok := langpack.CanonicalLang(system); ok {
		return c
	}
	return "en"
}

func specificLang(code string) (string, bool) {
	c, ok := langpack.CanonicalLang(code)
	if !ok || c == "" || c == "en" {
		return "", false
	}
	return c, true
}

// baseKey strips the CHAT_ / CHANNEL_ peer prefix. PINNED_* and REACT_* stay
// as they are. CHAT_REACT_TEXT becomes REACT_TEXT.
func baseKey(loc string) string {
	switch {
	case strings.HasPrefix(loc, "CHANNEL_"):
		return strings.TrimPrefix(loc, "CHANNEL_")
	case strings.HasPrefix(loc, "CHAT_"):
		return strings.TrimPrefix(loc, "CHAT_")
	default:
		return loc
	}
}

// phraseFor is the plain alert body for locKey in lang, with no loc-args
// filled in. The sender name and chat title are the notification title and
// subtitle. Names and the message text are filled by formatPushBody.
func phraseFor(lang, locKey string) string {
	return formatPushBody(lang, locKey, nil, "", "")
}

// formatPushBody renders the iOS langpack string PUSH_<locKey>.
// Telegram splits that string on "|": the left side is the title, the right
// side is the body. Placeholders are loc-args. A name that is already the
// title or subtitle is removed from the front of the body so a private chat
// reads "🎤 语音消息" under the sender, not "林：🎤 语音消息" a second time.
func formatPushBody(lang, locKey string, args []string, title, subtitle string) string {
	tmpl := pushTemplate(lang, locKey)
	body := fillPushTemplate(tmpl, args)
	return trimShownName(body, subtitle, title)
}

func pushTemplate(lang, locKey string) string {
	key := "PUSH_" + locKey
	code := NormalizeLang(lang)
	if cat := langpack.DefaultCatalog(); cat != nil {
		if s, ok := cat.Plain("ios", code, key); ok {
			return afterPipe(s)
		}
		if code != "en" {
			if s, ok := cat.Plain("ios", "en", key); ok {
				return afterPipe(s)
			}
		}
	}
	if s, ok := discoverPhrase(code, baseKey(locKey)); ok {
		return s
	}
	if s, ok := enPhrases[baseKey(locKey)]; ok {
		return s
	}
	return "sent you a message"
}

func afterPipe(s string) string {
	if i := strings.Index(s, "|"); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return strings.TrimSpace(s)
}

func fillPushTemplate(tmpl string, args []string) string {
	for i := len(args); i >= 1; i-- {
		tmpl = strings.ReplaceAll(tmpl, "%"+strconv.Itoa(i)+"$@", args[i-1])
	}
	for {
		i := strings.Index(tmpl, "%")
		if i < 0 {
			break
		}
		j := strings.Index(tmpl[i:], "$@")
		if j < 0 {
			break
		}
		tmpl = tmpl[:i] + tmpl[i+j+2:]
	}
	return strings.Trim(strings.TrimSpace(tmpl), "：:|-–— ")
}

func trimShownName(body, subtitle, title string) string {
	body = strings.TrimSpace(body)
	for _, name := range []string{subtitle, title} {
		name = strings.TrimSpace(name)
		if name == "" || !strings.HasPrefix(body, name) || len(body) == len(name) {
			continue
		}
		rest := body[len(name):]
		r, _ := utf8.DecodeRuneInString(rest)
		// "林：🎤 语音消息" repeats the sender already shown as the title.
		// A message whose text happens to start with that name is left as written.
		if r != '：' && r != ':' && r != '|' {
			continue
		}
		rest = strings.Trim(strings.TrimSpace(rest), "：:|-–— ")
		if rest != "" {
			body = rest
		}
	}
	return strings.TrimSpace(body)
}

// enPhrases are the bodies previously hard-coded in previewBody.
var enPhrases = map[string]string{
	"MESSAGE_NOTEXT":       "sent you a message",
	"MESSAGE_PHOTO":        "sent you a photo",
	"MESSAGE_PHOTO_SECRET": "sent you a self-destructing photo",
	"MESSAGE_VIDEO":        "sent you a video",
	"MESSAGE_VIDEO_SECRET": "sent you a self-destructing video",
	"MESSAGE_GIF":          "sent you a GIF",
	"MESSAGE_ROUND":        "sent you a video message",
	"MESSAGE_AUDIO":        "sent you a voice message",
	"MESSAGE_STICKER":      "sent you a sticker",
	"MESSAGE_DOC":          "sent you a file",
	"MESSAGE_GEO":          "sent you a map",
	"MESSAGE_GEOLIVE":      "started sharing their live location",
	"MESSAGE_CONTACT":      "shared a contact",
	"MESSAGE_POLL":         "sent you a poll",
	"MESSAGE_QUIZ":         "sent you a quiz",
	"PINNED_NOTEXT":        "pinned a message",
	"PINNED_TEXT":          "pinned a message",
	"PINNED_PHOTO":         "pinned a photo",
	"PINNED_VIDEO":         "pinned a video",
	"PINNED_GIF":           "pinned a GIF",
	"PINNED_ROUND":         "pinned a video message",
	"PINNED_AUDIO":         "pinned a voice message",
	"PINNED_STICKER":       "pinned a sticker",
	"PINNED_DOC":           "pinned a file",
	"PINNED_GEO":           "pinned a map",
	"PINNED_GEOLIVE":       "pinned a live location",
	"PINNED_CONTACT":       "pinned a contact",
	"PINNED_POLL":          "pinned a poll",
	// Reaction pushes are not emitted (updateMessageReactions has no single
	// actor). The phrases are here so a REACT_* loc-key still renders.
	"REACT_NOTEXT":  "reacted to your message",
	"REACT_TEXT":    "reacted to your message",
	"REACT_PHOTO":   "reacted to your photo",
	"REACT_VIDEO":   "reacted to your video",
	"REACT_GIF":     "reacted to your GIF",
	"REACT_ROUND":   "reacted to your video message",
	"REACT_AUDIO":   "reacted to your voice message",
	"REACT_STICKER": "reacted to your sticker",
	"REACT_DOC":     "reacted to your file",
	"REACT_GEO":     "reacted to your map",
	"REACT_GEOLIVE": "reacted to your live location",
	"REACT_CONTACT": "reacted to your contact",
	"REACT_POLL":    "reacted to your poll",
}
