package usernamepolicy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/contentsafety"
	"google.golang.org/grpc/status"
)

func TestRejectDisplay(t *testing.T) {
	ctx := context.Background()
	t.Setenv(EnvEnabled, "")
	if err := RejectDisplay(ctx, mtproto.PEER_USER, 7, "admin"); err != nil {
		t.Fatal(err)
	}

	t.Setenv(EnvEnabled, "1")
	for _, name := range []string{"Ada", "小明", "support", "Book Club", ""} {
		if err := RejectDisplay(ctx, mtproto.PEER_USER, 7, name); err != nil {
			t.Fatalf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"admin", "ADMIN", "tellomi", "admin_01", "Tel1omi"} {
		err := RejectDisplay(ctx, mtproto.PEER_USER, 7, name)
		if status.Convert(err).Message() != "FIRSTNAME_INVALID" {
			t.Fatalf("%q: %v", name, err)
		}
		if strings.Contains(err.Error(), "rule") || strings.Contains(err.Error(), "brand") || strings.Contains(err.Error(), "impersonat") {
			t.Fatalf("%q leaked rule info: %v", name, err)
		}
	}
	if err := RejectDisplay(ctx, mtproto.PEER_USER, 777000, "admin"); err != nil {
		t.Fatalf("official admin display: %v", err)
	}
	if err := RejectDisplay(ctx, mtproto.PEER_USER, 777001, "tellomi"); err != nil {
		t.Fatalf("official tellomi display: %v", err)
	}
	if err := RejectDisplay(ctx, mtproto.PEER_USER, 777000, "admin_01"); err == nil {
		t.Fatal("official variant was allowed")
	}
	if err := RejectDisplay(ctx, mtproto.PEER_CHANNEL, 9, "tellomi"); status.Convert(err).Message() != "FIRSTNAME_INVALID" {
		t.Fatalf("channel title: %v", err)
	}
}

func TestRejectDisplayContentSafety(t *testing.T) {
	ctx := context.Background()
	t.Setenv(EnvEnabled, "")
	t.Setenv("TELLOMI_CONTENT_SAFETY", "aliyun")
	t.Setenv("TELLOMI_CONTENT_SAFETY_ACCESS_KEY_ID", "id")
	t.Setenv("TELLOMI_CONTENT_SAFETY_ACCESS_KEY_SECRET", "secret")

	block := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Code":200,"Data":{"labels":"political_content"},"RequestId":"x"}`))
	}))
	defer block.Close()
	t.Setenv("TELLOMI_CONTENT_SAFETY_ENDPOINT", block.URL)
	err := RejectDisplay(ctx, mtproto.PEER_USER, 7, "小明")
	if status.Convert(err).Message() != "FIRSTNAME_INVALID" {
		t.Fatalf("blocked nickname: %v", err)
	}
	if strings.Contains(err.Error(), "political") || strings.Contains(err.Error(), "小明") {
		t.Fatalf("error leaked the decision: %v", err)
	}
	if err := RejectDisplay(ctx, mtproto.PEER_USER, 7, "Ada"); err != nil {
		t.Fatalf("ascii must stay on the local lexicon: %v", err)
	}

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer down.Close()
	t.Setenv("TELLOMI_CONTENT_SAFETY_ENDPOINT", down.URL)
	if err := RejectDisplay(ctx, mtproto.PEER_USER, 7, "小明"); err != nil {
		t.Fatalf("outage must allow the name: %v", err)
	}
	if contentsafety.Decide(ctx, "小明") != contentsafety.Fallback {
		t.Fatal("outage was not a fallback")
	}
}

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
