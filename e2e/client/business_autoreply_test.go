package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestBusinessProfileAndAutoReply covers ISSUE-S-BIZ-AUTOREPLY-001 (row 85)
// and ISSUE-S-BIZ-PERSIST-001 (row 86). Greeting and away shortcuts are sent
// when someone messages the business account, and the saved profile is what
// users.getFullUser returns.
func TestBusinessProfileAndAutoReply(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	biz, _ := dialPhone(t, ctx, cfg, "9996610433", "BizShop")
	customer, _ := dialPhone(t, ctx, cfg, "9996610434", "BizGuest")

	greetID := saveShortcut(t, ctx, biz, "greet", "welcome-from-shop")
	awayID := saveShortcut(t, ctx, biz, "away", "away-from-shop")
	recipients := mtproto.MakeTLInputBusinessRecipients(&mtproto.InputBusinessRecipients{
		NonContacts: true,
	}).To_InputBusinessRecipients()
	if _, err := biz.Invoke(ctx, &mtproto.TLAccountUpdateBusinessGreetingMessage{
		Constructor: mtproto.CRC32_account_updateBusinessGreetingMessage,
		Message: mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
			ShortcutId:     greetID,
			NoActivityDays: 7,
			Recipients:     recipients,
		}).To_InputBusinessGreetingMessage(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := biz.Invoke(ctx, &mtproto.TLAccountUpdateBusinessAwayMessage{
		Constructor: mtproto.CRC32_account_updateBusinessAwayMessage,
		Message: mtproto.MakeTLInputBusinessAwayMessage(&mtproto.InputBusinessAwayMessage{
			ShortcutId: awayID,
			Schedule:   mtproto.MakeTLBusinessAwayMessageScheduleAlways(nil).To_BusinessAwayMessageSchedule(),
			Recipients: recipients,
		}).To_InputBusinessAwayMessage(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := biz.Invoke(ctx, &mtproto.TLAccountUpdateBusinessLocation{
		Constructor: mtproto.CRC32_account_updateBusinessLocation,
		GeoPoint:    mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 31.2, Long: 121.5}).To_InputGeoPoint(),
		Address:     wrapperspb.String("Nanjing Rd"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := biz.Invoke(ctx, &mtproto.TLAccountUpdateBusinessIntro{
		Constructor: mtproto.CRC32_account_updateBusinessIntro,
		Intro: mtproto.MakeTLInputBusinessIntro(&mtproto.InputBusinessIntro{
			Title:       "Shop",
			Description: "Open daily",
		}).To_InputBusinessIntro(),
	}); err != nil {
		t.Fatal(err)
	}
	full := fullUserOf(t, invokeOK(t, biz, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUserSelf(),
	}))
	if full.GetBusinessLocation().GetAddress() != "Nanjing Rd" || full.GetBusinessIntro().GetTitle() != "Shop" {
		t.Fatalf("saved profile location=%q intro=%q", full.GetBusinessLocation().GetAddress(), full.GetBusinessIntro().GetTitle())
	}
	if full.GetBusinessGreetingMessage().GetShortcutId() != greetID || full.GetBusinessAwayMessage().GetShortcutId() != awayID {
		t.Fatalf("saved shortcuts greet=%d away=%d", full.GetBusinessGreetingMessage().GetShortcutId(), full.GetBusinessAwayMessage().GetShortcutId())
	}

	bizID, bizHash, err := customer.ImportPhone(ctx, "9996610433", "BizShop")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerUser(bizID, bizHash)
	if _, err = customer.SendText(ctx, peer, "hello shop", nil); err != nil {
		t.Fatal(err)
	}
	msgs := waitHistory(t, ctx, customer, peer, "welcome-from-shop", "away-from-shop")
	if countText(msgs, "welcome-from-shop") != 1 || countText(msgs, "away-from-shop") != 1 {
		t.Fatalf("auto replies %#v", texts(msgs))
	}
	if _, err = customer.SendText(ctx, peer, "hello again", nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	again, err := customer.History(ctx, peer, 20)
	if err != nil {
		t.Fatal(err)
	}
	if countText(again, "welcome-from-shop") != 1 || countText(again, "away-from-shop") != 1 {
		t.Fatalf("resent %#v", texts(again))
	}
}

func saveShortcut(t *testing.T, ctx context.Context, c *Client, name, text string) int32 {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerSelf(),
		Message:     text,
		RandomId:    randomID(),
		QuickReplyShortcut: mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{
			Shortcut: name,
		}).To_InputQuickReplyShortcut(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, up := range updatesOf(obj) {
		if q := up.GetQuickReply(); q != nil && q.GetShortcutId() != 0 {
			return q.GetShortcutId()
		}
	}
	t.Fatal("shortcut id missing")
	return 0
}

func waitHistory(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, want ...string) []*mtproto.Message {
	t.Helper()
	var last []*mtproto.Message
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		msgs, err := c.History(ctx, peer, 20)
		if err != nil {
			t.Fatal(err)
		}
		last = msgs
		ok := true
		for _, text := range want {
			if !hasText(msgs, text) {
				ok = false
				break
			}
		}
		if ok {
			return msgs
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("history missing %q, saw %v", strings.Join(want, ","), texts(last))
	return nil
}

func countText(msgs []*mtproto.Message, text string) int {
	n := 0
	for _, m := range msgs {
		if m != nil && m.GetMessage() == text {
			n++
		}
	}
	return n
}

func texts(msgs []*mtproto.Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m != nil && m.GetMessage() != "" {
			out = append(out, m.GetMessage())
		}
	}
	return out
}
