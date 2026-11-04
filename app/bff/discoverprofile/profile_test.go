package discoverprofile

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func rules(names ...string) []Rule {
	out := make([]Rule, len(names))
	for i, name := range names {
		out[i] = Rule{Name: name}
	}
	return out
}

func TestFieldVisibleMatchesProfileOptions(t *testing.T) {
	stranger := Audience{}
	contact := Audience{Contact: true}
	friend := Audience{Contact: true, CloseFriend: true}
	viewer := int64(2)

	everyone := rules(mtproto.Predicate_privacyValueAllowAll)
	contacts := rules(mtproto.Predicate_privacyValueAllowContacts)
	nobody := rules(mtproto.Predicate_privacyValueDisallowAll)
	closeOnly := rules(mtproto.Predicate_privacyValueAllowCloseFriends)

	if !FieldVisible(nil, viewer, stranger) || !FieldVisible(everyone, viewer, stranger) {
		t.Fatal("default and everyone must show the field")
	}
	if FieldVisible(contacts, viewer, stranger) || !FieldVisible(contacts, viewer, contact) || !FieldVisible(contacts, viewer, friend) {
		t.Fatal("contacts")
	}
	if FieldVisible(nobody, viewer, stranger) || FieldVisible(nobody, viewer, friend) {
		t.Fatal("nobody")
	}
	if FieldVisible(closeOnly, viewer, contact) || !FieldVisible(closeOnly, viewer, friend) || FieldVisible(closeOnly, viewer, stranger) {
		t.Fatal("close friends")
	}

	except := []Rule{
		{Name: mtproto.Predicate_privacyValueAllowAll},
		{Name: mtproto.Predicate_privacyValueDisallowUsers, Users: []int64{viewer}},
	}
	if FieldVisible(except, viewer, stranger) || !FieldVisible(except, 3, stranger) {
		t.Fatal("disallow one user")
	}
	picked := []Rule{
		{Name: mtproto.Predicate_privacyValueDisallowAll},
		{Name: mtproto.Predicate_privacyValueAllowUsers, Users: []int64{viewer}},
	}
	if !FieldVisible(picked, viewer, stranger) || FieldVisible(picked, 3, stranger) {
		t.Fatal("allow one user")
	}
	mixed := []Rule{
		{Name: mtproto.Predicate_privacyValueAllowAll},
		{Name: mtproto.Predicate_privacyValueDisallowContacts},
	}
	if FieldVisible(mixed, viewer, contact) || !FieldVisible(mixed, viewer, stranger) {
		t.Fatal("disallow contacts")
	}
	chatOnly := rules(mtproto.Predicate_privacyValueAllowChatParticipants)
	if FieldVisible(chatOnly, viewer, contact) {
		t.Fatal("chat participants are not applied")
	}
}

func TestCardHidesPhotoAndAbout(t *testing.T) {
	prof := Profile{FirstName: "林", LastName: "安", Username: "lin", About: "你好", PhotoID: 9001}
	priv := Privacy{
		Photo:    rules(mtproto.Predicate_privacyValueAllowContacts),
		PhotoSet: true,
		About:    rules(mtproto.Predicate_privacyValueAllowContacts),
		AboutSet: true,
	}
	stranger := Card(2, 1, prof, priv, Audience{})
	if stranger.FirstName != "林" || stranger.Username != "lin" || stranger.Photo || stranger.PhotoID != "" || stranger.AboutVisible || stranger.About != "" {
		t.Fatalf("stranger %+v", stranger)
	}
	contact := Card(2, 1, prof, priv, Audience{Contact: true})
	if !contact.Photo || contact.PhotoID != "9001" || !contact.AboutVisible || contact.About != "你好" {
		t.Fatalf("contact %+v", contact)
	}
	self := Card(1, 1, prof, Privacy{Photo: rules(mtproto.Predicate_privacyValueDisallowAll), PhotoSet: true, PhotoBroken: true}, Audience{})
	if !self.Photo || self.PhotoID != "9001" || !self.AboutVisible {
		t.Fatalf("self %+v", self)
	}
	deleted := Card(2, 1, Profile{FirstName: "林", PhotoID: 9001, Deleted: true}, priv, Audience{Contact: true})
	if !deleted.Deleted || deleted.FirstName != "" || deleted.Photo || deleted.PhotoID != "" {
		t.Fatalf("deleted %+v", deleted)
	}
	broken := Card(2, 1, prof, Privacy{PhotoSet: true, PhotoBroken: true}, Audience{Contact: true})
	if broken.Photo || broken.FirstName != "林" {
		t.Fatalf("broken %+v", broken)
	}
	plain := Card(2, 1, prof, Privacy{}, Audience{})
	if !plain.Photo || !plain.AboutVisible || plain.About != "你好" {
		t.Fatalf("default %+v", plain)
	}
}

func TestParseStoredRules(t *testing.T) {
	stored := []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueDisallowUsers(&mtproto.PrivacyRule{Users: []int64{42}}).To_PrivacyRule(),
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	got, broken := ParseRules(string(raw))
	if broken || len(got) != 2 || got[0].Name != mtproto.Predicate_privacyValueAllowContacts || !contains(got[1].Users, 42) {
		t.Fatalf("%s → %+v broken %v", raw, got, broken)
	}
	if _, broken = ParseRules(""); broken {
		t.Fatal("empty is unset")
	}
	if _, broken = ParseRules("{"); !broken {
		t.Fatal("bad json")
	}
}

func TestQueriesOmitSecrets(t *testing.T) {
	all := userLookupSQL + profileSQL + privacySQL + edgeSQL
	for _, word := range []string{"phone", "contact_first_name", "access_hash", "secret"} {
		if strings.Contains(strings.ToLower(all), word) {
			t.Fatalf("%s leaked into %s", word, all)
		}
	}
	if !strings.Contains(edgeSQL, "is_deleted = 0") || !strings.Contains(edgeSQL, "close_friend") {
		t.Fatal(edgeSQL)
	}
	if !strings.Contains(profileSQL, "first_name") || !strings.Contains(profileSQL, "photo_id") {
		t.Fatal(profileSQL)
	}
}
