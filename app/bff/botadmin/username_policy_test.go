package botadmin

import (
	"context"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/usernamepolicy"
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
