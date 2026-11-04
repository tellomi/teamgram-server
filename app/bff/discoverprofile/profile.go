// Package discoverprofile answers content-engine profile cards.
// The engine does not store nicknames or avatars. On each read it asks
// teamgram, and teamgram applies the author's current privacy rules (D-17).
package discoverprofile

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/teamgram/proto/mtproto"
)

const (
	// EnvEnabled turns the internal profile API on. Unset or any other value is off.
	EnvEnabled = "TELLOMI_DISCOVER_PROFILES"
	// EnvKey is standard base64 of the bearer secret the engine sends.
	// Decoded length is at least 32 bytes. It is not the relation key.
	EnvKey = "TELLOMI_DISCOVER_PROFILE_KEY"
	// EnvAddr is the listen address. Empty means 127.0.0.1:8816. "off" does not listen.
	EnvAddr = "TELLOMI_DISCOVER_PROFILE_ADDR"

	defaultAddr = "127.0.0.1:8816"
	maxAuthors  = 100

	keyProfilePhoto int32 = mtproto.PROFILE_PHOTO
	keyAbout        int32 = mtproto.ABOUT
)

// Profile is the account row. Contact-book names are not used.
type Profile struct {
	FirstName string
	LastName  string
	Username  string
	About     string
	PhotoID   int64
	Deleted   bool
}

// Rule is one stored privacy rule. Chat, premium, and bot rules are ignored.
type Rule struct {
	Name  string
	Users []int64
}

// Privacy is the author's profile-photo and about rules.
// A key that was never set uses Telegram's default: everyone can see it.
// A key whose JSON cannot be read is hidden from other people.
type Privacy struct {
	Photo       []Rule
	About       []Rule
	PhotoSet    bool
	AboutSet    bool
	PhotoBroken bool
	AboutBroken bool
}

// Audience is the viewer's place in the author's contacts.
// CloseFriend is only true when the contact row is still live.
type Audience struct {
	Contact     bool
	CloseFriend bool
}

// Result is one author in the request order.
// Phone numbers are never included.
type Result struct {
	Author       string `json:"author"`
	Known        bool   `json:"known"`
	Deleted      bool   `json:"deleted"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Username     string `json:"username"`
	About        string `json:"about"`
	AboutVisible bool   `json:"about_visible"`
	Photo        bool   `json:"photo"`
	PhotoID      string `json:"photo_id"`
}

// Answer is the HTTP body.
type Answer struct {
	ViewerKnown bool     `json:"viewer_known"`
	Results     []Result `json:"results"`
}

// ParseRules reads the JSON stored in user_privacies.rules.
// broken is true when the text is not a JSON array. An empty value is
// "not set", which the caller treats as the default.
func ParseRules(raw string) (rules []Rule, broken bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, false
	}
	var rows []struct {
		PredicateName string  `json:"predicate_name"`
		Users         []int64 `json:"users"`
	}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, true
	}
	out := make([]Rule, 0, len(rows))
	for _, row := range rows {
		out = append(out, Rule{Name: row.PredicateName, Users: row.Users})
	}
	return out, false
}

// FieldVisible applies the author's rules to one viewer.
// Self is handled by the caller. An empty list is "everyone".
//
// Order matches Telegram: a named disallow wins, then a named allow,
// then the base rule. disallowContacts removes contacts from allow-all
// and allow-contacts. Close friends are a live contact with the flag set.
// Chat-participant, premium, and bot rules are not applied: this API
// cannot see those sets, so they neither grant nor remove access.
func FieldVisible(rules []Rule, viewer int64, aud Audience) bool {
	if len(rules) == 0 {
		return true
	}
	for _, r := range rules {
		if r.Name == mtproto.Predicate_privacyValueDisallowUsers && contains(r.Users, viewer) {
			return false
		}
	}
	for _, r := range rules {
		if r.Name == mtproto.Predicate_privacyValueAllowUsers && contains(r.Users, viewer) {
			return true
		}
	}
	contactsDenied := false
	allowContacts := false
	allowClose := false
	allowAll := false
	disallowAll := false
	for _, r := range rules {
		switch r.Name {
		case mtproto.Predicate_privacyValueDisallowContacts:
			contactsDenied = true
		case mtproto.Predicate_privacyValueAllowContacts:
			allowContacts = true
		case mtproto.Predicate_privacyValueAllowCloseFriends:
			allowClose = true
		case mtproto.Predicate_privacyValueAllowAll:
			allowAll = true
		case mtproto.Predicate_privacyValueDisallowAll:
			disallowAll = true
		}
	}
	if disallowAll {
		return false
	}
	if aud.Contact && contactsDenied {
		return false
	}
	if allowContacts && aud.Contact {
		return true
	}
	if allowClose && aud.CloseFriend {
		return true
	}
	if allowAll {
		return true
	}
	return false
}

func contains(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func show(rules []Rule, set, broken, self bool, viewer int64, aud Audience) bool {
	if self {
		return true
	}
	if broken {
		return false
	}
	if !set {
		return true
	}
	return FieldVisible(rules, viewer, aud)
}

// Card is what one viewer may see of one author.
// A deleted account keeps no name, bio, username, or photo.
// The nickname is the account name, not a contact-book alias.
func Card(viewer, author int64, prof Profile, priv Privacy, aud Audience) Result {
	self := viewer > 0 && viewer == author
	if prof.Deleted {
		return Result{Known: true, Deleted: true}
	}
	if aud.CloseFriend {
		aud.Contact = true
	}
	out := Result{
		Known:     true,
		FirstName: prof.FirstName,
		LastName:  prof.LastName,
		Username:  prof.Username,
	}
	if show(priv.About, priv.AboutSet, priv.AboutBroken, self, viewer, aud) {
		out.AboutVisible = true
		out.About = prof.About
	}
	if show(priv.Photo, priv.PhotoSet, priv.PhotoBroken, self, viewer, aud) && prof.PhotoID > 0 {
		out.Photo = true
		out.PhotoID = formatID(prof.PhotoID)
	}
	return out
}

func formatID(id int64) string {
	if id <= 0 {
		return ""
	}
	return itoa(id)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	u := n
	if neg {
		u = -n
	}
	for u > 0 {
		i--
		b[i] = byte('0' + u%10)
		u /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Evaluate builds the response. Unknown authors stay empty.
// An unknown viewer is a stranger: public fields only.
func Evaluate(viewerSub string, viewerID int64, authors []string, users map[string]int64, profiles map[int64]Profile, privacy map[int64]Privacy, edges map[int64]Edge) Answer {
	out := Answer{
		ViewerKnown: viewerID > 0,
		Results:     make([]Result, len(authors)),
	}
	for i, author := range authors {
		row := Result{Author: author}
		authorID := users[author]
		if authorID <= 0 {
			out.Results[i] = row
			continue
		}
		prof, ok := profiles[authorID]
		if !ok {
			row.Known = true
			row.Deleted = true
			out.Results[i] = row
			continue
		}
		aud := Audience{}
		if viewerID > 0 && authorID != viewerID && author != viewerSub {
			aud = Audience{Contact: edges[authorID].Contact, CloseFriend: edges[authorID].CloseFriend}
		}
		row = Card(viewerID, authorID, prof, privacy[authorID], aud)
		row.Author = author
		out.Results[i] = row
	}
	return out
}

func subKey(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
