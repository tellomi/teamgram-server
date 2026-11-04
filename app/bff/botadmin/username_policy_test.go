package botadmin

import (
	"context"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/usernamepolicy"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBotUsernamePolicy(t *testing.T) {
	Reset()
	t.Setenv(usernamepolicy.EnvEnabled, "1")
	ctx := context.Background()

	if _, err := CheckUsername(ctx, 7, false, "newsbot"); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckUsername(ctx, 7, true, "wechat"); codeOf(err) != "USERNAME_OCCUPIED" {
		t.Fatalf("wechat: %s", codeOf(err))
	}
	if _, err := CheckUsername(ctx, 7, false, "admin_bot"); codeOf(err) != "USERNAME_OCCUPIED" {
		t.Fatalf("admin_bot: %s", codeOf(err))
	}
	if _, err := CreateBot(ctx, 7, false, "G", "google_bot", manager()); codeOf(err) != "USERNAME_OCCUPIED" {
		t.Fatalf("google_bot: %s", codeOf(err))
	}
	got, err := CreateBot(ctx, 7, false, "News", "newsbot", manager())
	if err != nil {
		t.Fatal(err)
	}
	in := asUser(got.GetId(), got.GetAccessHash().GetValue())
	if _, err := AddUsername(ctx, 7, in, "apple"); codeOf(err) != "USERNAME_OCCUPIED" {
		t.Fatalf("apple: %s", codeOf(err))
	}
	if _, err := AddUsername(ctx, 7, in, "extra"); err != nil {
		t.Fatal(err)
	}
}

func TestBotDisplayNamePolicy(t *testing.T) {
	Reset()
	t.Setenv(usernamepolicy.EnvEnabled, "1")
	ctx := context.Background()

	if _, err := CreateBot(ctx, 7, false, "admin", "adminbot", manager()); codeOf(err) != "FIRSTNAME_INVALID" {
		t.Fatalf("create admin: %s", codeOf(err))
	}
	got, err := CreateBot(ctx, 7, false, "News", "newsbot", manager())
	if err != nil {
		t.Fatal(err)
	}
	in := asUser(got.GetId(), got.GetAccessHash().GetValue())
	if _, err := SetBotInfo(ctx, 7, in, "", wrapperspb.String("tellomi"), nil, nil); codeOf(err) != "FIRSTNAME_INVALID" {
		t.Fatalf("rename tellomi: %s", codeOf(err))
	}
	if _, err := SetBotInfo(ctx, 7, in, "", wrapperspb.String("News"), wrapperspb.String("about"), nil); err != nil {
		t.Fatalf("unchanged name: %v", err)
	}
	if _, err := SetBotInfo(ctx, 7, in, "", wrapperspb.String("Ada"), nil, nil); err != nil {
		t.Fatalf("ada: %v", err)
	}
	if _, err := SetBotInfo(ctx, 7, in, "", nil, wrapperspb.String("tellomi"), nil); codeOf(err) != "FIRSTNAME_INVALID" {
		t.Fatalf("about tellomi: %s", codeOf(err))
	}
	if _, err := SetBotInfo(ctx, 7, in, "", nil, nil, wrapperspb.String("admin")); codeOf(err) != "FIRSTNAME_INVALID" {
		t.Fatalf("description admin: %s", codeOf(err))
	}
	if _, err := SetBotInfo(ctx, 7, in, "", nil, wrapperspb.String("answers questions"), wrapperspb.String("a small helper")); err != nil {
		t.Fatalf("normal bio: %v", err)
	}
	info, err := GetBotInfo(ctx, 7, in, "")
	if err != nil {
		t.Fatal(err)
	}
	if info.GetAbout() != "answers questions" || info.GetDescription() != "a small helper" {
		t.Fatalf("stored about %q description %q", info.GetAbout(), info.GetDescription())
	}
}
