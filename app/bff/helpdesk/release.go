// Package helpdesk is the client-info surface behind the help.* methods
// that official apps call after login: app updates, changelogs, invite
// text, suggestions, CDN keys, and client logs.
package helpdesk

import (
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Release is one published app build.
type Release struct {
	Version    string
	Text       string
	URL        string
	Source     string // empty matches every installer source
	CannotSkip bool
}

// Note is one changelog entry newer clients should see once.
type Note struct {
	Version string
	Text    string
}

// AppUpdate compares the client's version with the published release.
// help.noAppUpdate is the documented result when nothing newer is published
// or the client is already on that version.
// https://core.telegram.org/method/help.getAppUpdate
func AppUpdate(clientVersion, source string, published *Release) *mtproto.Help_AppUpdate {
	if published == nil || strings.TrimSpace(published.Version) == "" {
		return noUpdate()
	}
	if published.Source != "" && !strings.EqualFold(strings.TrimSpace(published.Source), strings.TrimSpace(source)) {
		return noUpdate()
	}
	if !newer(published.Version, clientVersion) {
		return noUpdate()
	}
	text := strings.TrimSpace(published.Text)
	if text == "" {
		text = "A new version of Tellomi is available."
	}
	data := &mtproto.Help_AppUpdate{
		Id:         versionID(published.Version),
		Version:    published.Version,
		Text:       text,
		Entities:   []*mtproto.MessageEntity{},
		CanNotSkip: published.CannotSkip,
	}
	if u := strings.TrimSpace(published.URL); u != "" {
		data.Url = wrapperspb.String(u)
	}
	return mtproto.MakeTLHelpAppUpdate(data).To_Help_AppUpdate()
}

// LoadRelease reads the optional published update. An empty version means
// the server is not offering an update.
func LoadRelease() *Release {
	version := strings.TrimSpace(os.Getenv("TELLOMI_APP_UPDATE_VERSION"))
	if version == "" {
		return nil
	}
	return &Release{
		Version:    version,
		Text:       strings.TrimSpace(os.Getenv("TELLOMI_APP_UPDATE_TEXT")),
		URL:        strings.TrimSpace(os.Getenv("TELLOMI_APP_UPDATE_URL")),
		Source:     strings.TrimSpace(os.Getenv("TELLOMI_APP_UPDATE_SOURCE")),
		CannotSkip: os.Getenv("TELLOMI_APP_UPDATE_CANNOT_SKIP") == "1",
	}
}

// Changelog returns service notifications for notes newer than prev.
// An empty prev version, or a client already on the newest note, returns
// an updates constructor with no updates.
// https://core.telegram.org/method/help.getAppChangelog
func Changelog(prev string, notes []Note, now int32) *mtproto.Updates {
	var texts []string
	for _, note := range notes {
		if strings.TrimSpace(note.Version) == "" || strings.TrimSpace(note.Text) == "" {
			continue
		}
		if prev != "" && !newer(note.Version, prev) {
			continue
		}
		if prev == "" {
			continue
		}
		texts = append(texts, note.Text)
	}
	updates := make([]*mtproto.Update, 0, len(texts))
	for _, text := range texts {
		updates = append(updates, mtproto.MakeTLUpdateServiceNotification(&mtproto.Update{
			Popup:          true,
			Type:           "message",
			Message_STRING: text,
			Media:          mtproto.MakeTLMessageMediaEmpty(&mtproto.MessageMedia{}).To_MessageMedia(),
			Entities:       []*mtproto.MessageEntity{},
			InboxDate:      wrapperspb.Int32(now),
		}).To_Update())
	}
	users := []*mtproto.User{}
	if len(updates) > 0 {
		users = append(users, mtproto.MakeTLUser(&mtproto.User{
			Id:        777000,
			FirstName: wrapperspb.String("Tellomi"),
		}).To_User())
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: updates,
		Users:   users,
		Chats:   []*mtproto.Chat{},
		Date:    now,
		Seq:     0,
	}).To_Updates()
}

// LoadNotes reads TELLOMI_APP_CHANGELOG as "version|text" entries separated by ';'.
func LoadNotes() []Note {
	raw := strings.TrimSpace(os.Getenv("TELLOMI_APP_CHANGELOG"))
	if raw == "" {
		return nil
	}
	var notes []Note
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		ver, text, ok := strings.Cut(part, "|")
		if !ok {
			continue
		}
		notes = append(notes, Note{Version: strings.TrimSpace(ver), Text: strings.TrimSpace(text)})
	}
	return notes
}

// InviteText is the localized SMS/share invitation.
// https://core.telegram.org/method/help.getInviteText
func InviteText(lang string) string {
	link := strings.TrimRight(brand.LinkBase(), "/")
	switch normalizeLang(lang) {
	case "zh-hant":
		return "我在用 Tellomi 聊天，來加入我：" + link
	case "zh-hans":
		return "我在用 Tellomi 聊天，来加入我：" + link
	default:
		return "I'm on Tellomi. Join me: " + link
	}
}

// SupportName is the localized label for the support account.
// A loaded profile name wins. Otherwise the brand name is used.
// https://core.telegram.org/method/help.getSupportName
func SupportName(lang, profileName string) string {
	if name := strings.TrimSpace(profileName); name != "" {
		return name
	}
	switch normalizeLang(lang) {
	case "zh-hans":
		return "Tellomi 支持"
	case "zh-hant":
		return "Tellomi 支援"
	default:
		return "Tellomi Support"
	}
}

func noUpdate() *mtproto.Help_AppUpdate {
	return mtproto.MakeTLHelpNoAppUpdate(&mtproto.Help_AppUpdate{}).To_Help_AppUpdate()
}

func normalizeLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	switch {
	case strings.HasPrefix(lang, "zh-hant"), strings.HasPrefix(lang, "zh-tw"), strings.HasPrefix(lang, "zh-hk"), strings.HasPrefix(lang, "zh-mo"):
		return "zh-hant"
	case strings.HasPrefix(lang, "zh"):
		return "zh-hans"
	case strings.HasPrefix(lang, "en"):
		return "en"
	default:
		return lang
	}
}

// newer reports whether next is a strictly newer dotted version than prev.
// Missing prev is not newer. Non-numeric tails are ignored.
func newer(next, prev string) bool {
	a := parts(next)
	b := parts(prev)
	if len(a) == 0 {
		return false
	}
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av != bv {
			return av > bv
		}
	}
	return false
}

func parts(v string) []int {
	var out []int
	cur := -1
	started := false
	flush := func() {
		if cur >= 0 {
			out = append(out, cur)
			cur = -1
		}
	}
	for _, r := range v {
		if unicode.IsDigit(r) {
			started = true
			if cur < 0 {
				cur = 0
			}
			cur = cur*10 + int(r-'0')
			continue
		}
		if !started {
			continue
		}
		if r == '.' {
			flush()
			continue
		}
		break
	}
	flush()
	return out
}

func versionID(version string) int32 {
	var n int32
	for _, p := range parts(version) {
		n = n*1000 + int32(p)
	}
	if n == 0 {
		return 1
	}
	return n
}

// VersionInSource returns a dotted version embedded in source, if any.
func VersionInSource(source string) string {
	ps := parts(source)
	if len(ps) == 0 {
		return ""
	}
	ss := make([]string, len(ps))
	for i, p := range ps {
		ss[i] = strconv.Itoa(p)
	}
	return strings.Join(ss, ".")
}
