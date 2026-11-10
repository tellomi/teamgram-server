package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestLinksUseTellCC(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	a.SetProfile(ProfileIOS)
	obj := invokeOK(t, a, &mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
	cfg, ok := obj.(*mtproto.TLConfig)
	if !ok {
		t.Fatalf("help.getConfig returned %T", obj)
	}
	prefix := cfg.GetMeUrlPrefix()
	if !strings.Contains(prefix, "tell.cc") {
		t.Fatalf("me_url_prefix = %q, want a tell.cc link", prefix)
	}
	for _, banned := range []string{"t.me", "telegram.me", "telegram.dog"} {
		if strings.Contains(prefix, banned) {
			t.Fatalf("me_url_prefix %q still points at %s", prefix, banned)
		}
	}

	app := invokeOK(t, a, &mtproto.TLHelpGetAppConfig61E3F854{Constructor: mtproto.CRC32_help_getAppConfig61E3F854})
	dump := fmt.Sprintf("%+v", app)
	if strings.Contains(dump, "https://t.me") || strings.Contains(dump, "https://telegram.me") {
		t.Fatalf("help.getAppConfig still advertises a Telegram short link: %s", dump)
	}
}

func TestSendMediaUserCardSurvives(t *testing.T) {
	a, b, _, _ := sharedPair(t)
	var name string
	for i := 0; i < 8; i++ {
		candidate := randomUsername()
		ctx, cancel := contextTimeout(20 * time.Second)
		_, err := a.Invoke(ctx, &mtproto.TLAccountUpdateUsername{
			Constructor: mtproto.CRC32_account_updateUsername,
			Username:    candidate,
		})
		cancel()
		if err != nil {
			if rpc, ok := asRPCError(err); ok && rpc.Message == "USERNAME_OCCUPIED" {
				continue
			}
			t.Fatal(err)
		}
		name = candidate
		break
	}
	if name == "" {
		t.Fatal("could not claim a username")
	}
	for _, raw := range []string{"https://t.me/" + name, "https://tell.cc/" + name} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			ctx, cancel := contextTimeout(30 * time.Second)
			defer cancel()
			res, err := a.Invoke(ctx, &mtproto.TLMessagesSendMedia{
				Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
				Peer:        inputPeerUser(bobSelf.ID, bobSelf.AccessHash),
				Media: mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{
					Url: raw,
				}).To_InputMedia(),
				Message:  raw,
				RandomId: randomID(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if !cardIn(allMessages(res), raw, name) {
				t.Fatalf("sender update missing telegram_user for %s", raw)
			}
			for _, side := range []struct {
				name   string
				client *Client
				peer   *mtproto.InputPeer
			}{
				{"sender", a, inputPeerUser(bobSelf.ID, bobSelf.AccessHash)},
				{"recipient", b, inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)},
			} {
				err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
					msgs, err := side.client.History(ctx, side.peer, 30)
					if err != nil {
						return false, err
					}
					return cardIn(msgs, raw, name), nil
				})
				if err != nil {
					t.Fatalf("%s history never showed %s: %v", side.name, raw, err)
				}
			}
		})
	}
}

func cardIn(msgs []*mtproto.Message, raw, username string) bool {
	for _, msg := range msgs {
		if msg == nil || !strings.Contains(msg.GetMessage(), raw) {
			continue
		}
		page := msg.GetMedia().GetWebpage()
		if page.GetType().GetValue() != "telegram_user" {
			continue
		}
		if strings.Contains(page.GetUrl_STRING(), username) || strings.Contains(page.GetDisplayUrl(), username) {
			return true
		}
	}
	return false
}
