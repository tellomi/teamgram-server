package helpdesk

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAppUpdateAndChangelog(t *testing.T) {
	none := AppUpdate("14.0.1", "android", nil)
	if none.GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
		t.Fatal(none.GetPredicateName())
	}
	rel := &Release{Version: "14.0.2", Text: "Fixes", URL: "https://tellomi.cn/get", Source: "android"}
	got := AppUpdate("14.0.1", "android", rel)
	if got.GetPredicateName() != mtproto.Predicate_help_appUpdate || got.GetVersion() != "14.0.2" || !newer("14.0.2", "14.0.1") {
		t.Fatalf("%#v", got)
	}
	if AppUpdate("14.0.2", "android", rel).GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
		t.Fatal("same version")
	}
	if AppUpdate("14.0.0", "ios", rel).GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
		t.Fatal("source")
	}
	if VersionInSource("android") != "" || VersionInSource("build 14.0.3") != "14.0.3" {
		t.Fatal(VersionInSource("build 14.0.3"))
	}

	empty := Changelog("", []Note{{Version: "14.0.0", Text: "Hello"}}, 10)
	if len(empty.GetUpdates()) != 0 {
		t.Fatal("empty prev")
	}
	notes := Changelog("13.9.0", []Note{{Version: "14.0.0", Text: "Hello Tellomi"}}, 10)
	if len(notes.GetUpdates()) != 1 || notes.GetUpdates()[0].GetMessage_STRING() != "Hello Tellomi" || notes.GetUsers()[0].GetId() != 777000 {
		t.Fatalf("%#v", notes)
	}
	if len(Changelog("14.0.0", []Note{{Version: "14.0.0", Text: "Hello"}}, 10).GetUpdates()) != 0 {
		t.Fatal("already seen")
	}
}

func TestInviteSupportSuggestionsLog(t *testing.T) {
	if InviteText("zh-CN") == InviteText("en") || InviteText("zh-TW") == InviteText("zh-CN") {
		t.Fatal(InviteText("zh-CN"))
	}
	if SupportName("en", "") != "Tellomi Support" || SupportName("zh", "Ada") != "Ada" {
		t.Fatal(SupportName("zh", "Ada"))
	}

	s := NewMemory([]string{"VALIDATE_PASSWORD", "AUTOARCHIVE_POPULAR"})
	ctx := context.Background()
	self := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	pending, dismissed := s.Lists(ctx, 4, self)
	if len(pending) != 2 || len(dismissed) != 0 {
		t.Fatalf("pending %#v dismissed %#v", pending, dismissed)
	}
	if err := s.Dismiss(ctx, 4, self, "VALIDATE_PASSWORD"); err != nil {
		t.Fatal(err)
	}
	pending, dismissed = s.Lists(ctx, 4, self)
	if len(pending) != 1 || pending[0] != "AUTOARCHIVE_POPULAR" || dismissed[0] != "VALIDATE_PASSWORD" {
		t.Fatalf("after %#v %#v", pending, dismissed)
	}
	if err := s.Dismiss(ctx, 4, self, "nope"); err != mtproto.ErrInputRequestInvalid {
		t.Fatal(err)
	}
	if err := s.Dismiss(ctx, 4, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{}).To_InputPeer(), "AUTOARCHIVE_POPULAR"); err != mtproto.ErrPeerIdInvalid {
		t.Fatal(err)
	}

	ResetRecent()
	err := Record(4, []*mtproto.InputAppEvent{{
		Time: 1, Type: "open", Peer: 8,
		Data: &mtproto.JSONValue{Value_STRING: "chat"},
	}})
	if err != nil || len(Recent()) != 1 || Recent()[0].Type != "open" {
		t.Fatal(err)
	}
	if err := Record(4, []*mtproto.InputAppEvent{{Type: ""}}); err != mtproto.ErrInputRequestInvalid {
		t.Fatal(err)
	}

	keys := parseKeys("2|-----BEGIN PUBLIC KEY-----\nMIIB\n-----END PUBLIC KEY-----")
	if len(keys) != 1 || keys[0].DC != 2 {
		t.Fatalf("%#v", keys)
	}
	if len(parseKeys("0|-----BEGIN PUBLIC KEY-----")) != 0 {
		t.Fatal("bad dc")
	}
}
