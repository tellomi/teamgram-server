package usernamepolicy

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestGateOffIsTransparent(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	if err := RejectUser(context.Background(), 7, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := RejectChannel(context.Background(), 9, "wechat"); err != nil {
		t.Fatal(err)
	}
	if err := RejectBot(context.Background(), 7, "telegram_support"); err != nil {
		t.Fatal(err)
	}
}

func TestGateOn(t *testing.T) {
	t.Setenv(EnvEnabled, "1")
	ctx := context.Background()

	if err := RejectUser(ctx, 7, "admin01"); err != nil {
		t.Fatal(err)
	}
	if err := RejectUser(ctx, 7, "support88"); err != nil {
		t.Fatal(err)
	}
	if err := RejectUser(ctx, 7, ""); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"admin", "admin_01", "support_88", "tellomi_xxx", "tel1omi", "wechat", "google", "apple", "telegram"} {
		err := RejectUser(ctx, 7, name)
		if status.Convert(err).Message() != "USERNAME_OCCUPIED" {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(err.Error(), "rule") || strings.Contains(err.Error(), "brand") || strings.Contains(err.Error(), "third") {
			t.Fatalf("%s leaked rule info: %v", name, err)
		}
	}

	if err := RejectUser(ctx, 777000, "admin"); err != nil {
		t.Fatalf("official admin: %v", err)
	}
	if err := RejectUser(ctx, 777000, "ADMIN"); err != nil {
		t.Fatalf("official ADMIN: %v", err)
	}
	if err := RejectUser(ctx, 777000, "admin_01"); err == nil {
		t.Fatal("official variant was allowed")
	}
	if err := RejectUser(ctx, 7, "admin"); err == nil {
		t.Fatal("stranger used admin")
	}
	if err := RejectUser(ctx, 777001, "tellomi"); err != nil {
		t.Fatalf("official tellomi: %v", err)
	}
	// news is not a policy term. The username table already holds it.
	// Policy still lets the official channel through, and still blocks
	// a reserved name on any other channel.
	if err := RejectChannel(ctx, 1073800001, "news"); err != nil {
		t.Fatalf("official news channel: %v", err)
	}
	if err := RejectChannel(ctx, 9, "google"); status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("stranger channel google: %v", err)
	}
	if err := RejectChannel(ctx, 9, "qq"); status.Convert(err).Message() != mtproto.ErrUsernameOccupied.Error() && status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("qq slug: %v", err)
	}
	if err := RejectBot(ctx, 7, "wechat"); status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("bot wechat: %v", err)
	}
	if err := RejectBot(ctx, 7, "newsbot"); err != nil {
		t.Fatalf("newsbot: %v", err)
	}
	// Official policy reserves help on slug only. A channel public name hits it.
	if err := RejectChannel(ctx, 9, "help"); status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("help slug: %v", err)
	}
}
