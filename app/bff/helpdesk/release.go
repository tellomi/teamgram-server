// Package helpdesk is the client-info surface behind the help.* methods
// that official apps call after login: app updates, changelogs, invite
// text, suggestions, CDN keys, and client logs.
package helpdesk

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"github.com/teamgram/teamgram-server/pkg/langpack"
	"github.com/teamgram/teamgram-server/pkg/usercopy"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Release is one published app build.
// Notes is lang_code to dialog text from latest.json "notes".
// Text is the string changelog, or TELLOMI_APP_UPDATE_TEXT.
type Release struct {
	Version    string
	Text       string
	Notes      map[string]string
	URL        string
	Source     string // empty matches every installer source
	CannotSkip bool
}

// Note is one changelog entry newer clients should see once.
type Note struct {
	Version string
	Text    string
}

// UpdateClient is the signed-in client's initConnection record.
// Known is false when lang_pack and the authorization row could not be loaded.
// LangCode and SystemLangCode choose the update sentence.
type UpdateClient struct {
	Known          bool
	LangPack       string
	LangCode       string
	SystemLangCode string
	DeviceModel    string
	SystemVersion  string
	AppVersion     string
}

// AppUpdateForSession answers help.getAppUpdate for one client.
// An iOS session always receives help.noAppUpdate. Every other known client,
// and a client whose session could not be loaded, is compared with the
// published release.
// https://core.telegram.org/method/help.getAppUpdate
func AppUpdateForSession(client UpdateClient, source string, published *Release) *mtproto.Help_AppUpdate {
	if client.Known && iosClient(client.LangPack, client.DeviceModel, client.SystemVersion) {
		return noUpdate()
	}
	version := strings.TrimSpace(client.AppVersion)
	if v := VersionInSource(source); v != "" {
		version = v
	}
	return AppUpdateForLang(version, source, client.language(), published)
}

func (c UpdateClient) language() string {
	// lang_code, then lang_pack, then system_lang_code. English is last.
	return langpack.ClientLang(c.LangCode, c.SystemLangCode, c.LangPack)
}

// iosClient reports a Telegram iOS session.
// lang_pack "ios" decides the platform. device_model and system_version apply
// when lang_pack is empty: iPhone, iPad, iPod, or a system version that
// starts with "iOS" or "iPadOS".
func iosClient(langPack, deviceModel, systemVersion string) bool {
	switch strings.ToLower(strings.TrimSpace(langPack)) {
	case "ios":
		return true
	case "":
	default:
		return false
	}
	model := strings.ToLower(strings.TrimSpace(deviceModel))
	if strings.Contains(model, "iphone") || strings.Contains(model, "ipad") || strings.Contains(model, "ipod") {
		return true
	}
	system := strings.ToLower(strings.TrimSpace(systemVersion))
	return strings.HasPrefix(system, "ios") || strings.HasPrefix(system, "ipados")
}

// AppUpdate compares the client's version with the published release.
// The dialog text is English. AppUpdateForLang picks the client's language.
// help.noAppUpdate is the documented result when nothing newer is published
// or the client is already on that version.
// A published version with no dot is a build number (for example 1800979),
// not a marketing version, and is treated as no update.
// https://core.telegram.org/method/help.getAppUpdate
func AppUpdate(clientVersion, source string, published *Release) *mtproto.Help_AppUpdate {
	return AppUpdateForLang(clientVersion, source, "", published)
}

// AppUpdateForLang is AppUpdate with the client's initConnection language.
// lang is lang_code, or already resolved by ClientLang. Empty is English.
func AppUpdateForLang(clientVersion, source, lang string, published *Release) *mtproto.Help_AppUpdate {
	if published == nil || strings.TrimSpace(published.Version) == "" {
		return noUpdate()
	}
	if bareBuildNumber(published.Version) {
		warnBareBuild(published.Version)
		return noUpdate()
	}
	if published.Source != "" && !strings.EqualFold(strings.TrimSpace(published.Source), strings.TrimSpace(source)) {
		return noUpdate()
	}
	if !newer(published.Version, clientVersion) {
		return noUpdate()
	}
	text := dialogText(lang, published)
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

// dialogText prefers a manifest note for the client language, then notes.en,
// then the string changelog (or TELLOMI_APP_UPDATE_TEXT). When none of those
// is set, the built-in sentence for that language is used. English is the
// fallback language.
func dialogText(lang string, published *Release) string {
	if published != nil {
		if t := noteForLang(published.Notes, lang); t != "" {
			return t
		}
		if t := strings.TrimSpace(published.Text); t != "" {
			return t
		}
	}
	return usercopy.AppUpdate(lang)
}

func noteForLang(notes map[string]string, lang string) string {
	if len(notes) == 0 {
		return ""
	}
	code := langpack.ClientLang(lang, "", "")
	if t := strings.TrimSpace(notes[code]); t != "" {
		return t
	}
	if code != "en" {
		if t := strings.TrimSpace(notes["en"]); t != "" {
			return t
		}
	}
	return ""
}

// LoadRelease reads the published Android update.
// A non-empty TELLOMI_APP_UPDATE_VERSION replaces the release manifest and
// is logged once. Leave it empty in production so latest.json drives the
// prompt. When it is empty, the manifest supplies versionName, the download
// URL, changelog, and notes. TEXT, URL, and SOURCE, when non-empty, replace
// those manifest fields. TEXT also drops notes, so that one string is the
// dialog in every language. The prompt is skippable unless CANNOT_SKIP is 1 or the
// manifest sets mandatory to true. An env-only release is mandatory only
// when CANNOT_SKIP is 1.
// A manifest that cannot be read, or an empty version, offers no update.
// TELLOMI_APP_UPDATE_VERSION is a dotted marketing version such as 14.0.4.
// A value with no dot is a build number and AppUpdate ignores it.
// iOS sessions ignore the release and always receive help.noAppUpdate.
func LoadRelease() *Release {
	if rel := releaseFromEnv(); rel != nil {
		return rel
	}
	rel, err := publishedManifest()
	if err != nil || rel == nil {
		return nil
	}
	return applyManifestOverrides(rel)
}

func releaseFromEnv() *Release {
	version := strings.TrimSpace(os.Getenv("TELLOMI_APP_UPDATE_VERSION"))
	if version == "" {
		return nil
	}
	warnEnvOverride(version)
	return &Release{
		Version:    version,
		Text:       strings.TrimSpace(os.Getenv("TELLOMI_APP_UPDATE_TEXT")),
		URL:        strings.TrimSpace(os.Getenv("TELLOMI_APP_UPDATE_URL")),
		Source:     strings.TrimSpace(os.Getenv("TELLOMI_APP_UPDATE_SOURCE")),
		CannotSkip: os.Getenv("TELLOMI_APP_UPDATE_CANNOT_SKIP") == "1",
	}
}

// applyManifestOverrides copies rel and replaces fields that the operator set.
// Empty TEXT, URL, and SOURCE leave the manifest value. CANNOT_SKIP=1 forces
// the dialog to stay up. Any other value leaves the manifest flag, which is
// set only when mandatory is the JSON boolean true.
func applyManifestOverrides(rel *Release) *Release {
	out := cloneRelease(rel)
	if text := strings.TrimSpace(os.Getenv("TELLOMI_APP_UPDATE_TEXT")); text != "" {
		out.Text = text
		out.Notes = nil
	}
	if url := strings.TrimSpace(os.Getenv("TELLOMI_APP_UPDATE_URL")); url != "" {
		out.URL = url
	}
	if source := strings.TrimSpace(os.Getenv("TELLOMI_APP_UPDATE_SOURCE")); source != "" {
		out.Source = source
	}
	if os.Getenv("TELLOMI_APP_UPDATE_CANNOT_SKIP") == "1" {
		out.CannotSkip = true
	}
	return out
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
	return usercopy.Invite(lang, link)
}

// SupportName is the localized label for the support account.
// A loaded profile name wins. Otherwise the brand name is used.
// https://core.telegram.org/method/help.getSupportName
func SupportName(lang, profileName string) string {
	return usercopy.Support(lang, profileName)
}

func noUpdate() *mtproto.Help_AppUpdate {
	return mtproto.MakeTLHelpNoAppUpdate(&mtproto.Help_AppUpdate{}).To_Help_AppUpdate()
}

// newer reports whether next is a strictly newer dotted marketing version than prev.
// A client string such as "14.0.4 (1801699) " compares as 14.0.4; the
// parenthetical build is ignored. next with no dot, such as 1800979, is a
// build number and is not newer. Missing prev is not newer.
func newer(next, prev string) bool {
	if bareBuildNumber(next) {
		return false
	}
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

// bareBuildNumber reports a version that is not a dotted marketing version.
func bareBuildNumber(version string) bool {
	return !strings.Contains(strings.TrimSpace(version), ".")
}

var (
	bareBuildLogged   sync.Map
	envOverrideLogged sync.Map
)

func warnBareBuild(version string) {
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}
	if _, loaded := bareBuildLogged.LoadOrStore(version, struct{}{}); loaded {
		return
	}
	logx.Errorf("help.getAppUpdate: published version %q has no dot and is a build number. Use a marketing version such as 14.0.4. This value is ignored. iOS clients always receive help.noAppUpdate.", version)
}

func warnEnvOverride(version string) {
	if _, loaded := envOverrideLogged.LoadOrStore(version, struct{}{}); loaded {
		return
	}
	logx.Errorf("help.getAppUpdate: TELLOMI_APP_UPDATE_VERSION %q overrides the Android release manifest. Leave it empty so latest.json drives the in-app prompt.", version)
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
