package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestBusinessProfileAndAutoReply covers ISSUE-S-BIZ-AUTOREPLY-001 (row 85)
// and ISSUE-S-BIZ-PERSIST-001 (row 86). A dialog with no earlier message gets
// the greeting. users.getFullUser returns the saved profile.
func TestBusinessProfileAndAutoReply(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	biz, _ := dialPhone(t, ctx, cfg, "9996610435", "BizShop2")
	customer, _ := dialPhone(t, ctx, cfg, "9996610436", "BizGuest2")

	stamp := time.Now().UnixNano()
	greetText := fmt.Sprintf("welcome-from-shop-%d", stamp)
	awayText := fmt.Sprintf("away-from-shop-%d", stamp)
	greetID := saveShortcut(t, ctx, biz, "greet", greetText)
	awayID := saveShortcut(t, ctx, biz, "away", awayText)
	recipients := mtproto.MakeTLInputBusinessRecipients(&mtproto.InputBusinessRecipients{
		NonContacts: true,
	}).To_InputBusinessRecipients()
	if _, err := biz.Invoke(ctx, &mtproto.TLAccountUpdateBusinessGreetingMessage{
		Constructor: mtproto.CRC32_account_updateBusinessGreetingMessage,
		Message: mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
			ShortcutId:     greetID,
			NoActivityDays: 0,
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

	bizID, bizHash, err := customer.ImportPhone(ctx, "9996610435", "BizShop2")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerUser(bizID, bizHash)
	before, err := customer.History(ctx, peer, 20)
	if err != nil {
		t.Fatal(err)
	}
	freshDialog := !hasPlainText(before)
	if _, err = customer.SendText(ctx, peer, fmt.Sprintf("hello-shop-%d", stamp), nil); err != nil {
		t.Fatal(err)
	}
	if freshDialog {
		msgs := waitHistory(t, ctx, customer, peer, greetText, awayText)
		if countText(msgs, greetText) != 1 || countText(msgs, awayText) != 1 {
			t.Fatalf("auto replies %#v", texts(msgs))
		}
		if _, err = customer.SendText(ctx, peer, fmt.Sprintf("hello-again-%d", stamp), nil); err != nil {
			t.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
		again, err := customer.History(ctx, peer, 30)
		if err != nil {
			t.Fatal(err)
		}
		if countText(again, greetText) != 1 || countText(again, awayText) != 1 {
			t.Fatalf("resent %#v", texts(again))
		}
		return
	}
	time.Sleep(500 * time.Millisecond)
	after, err := customer.History(ctx, peer, 30)
	if err != nil {
		t.Fatal(err)
	}
	if countText(after, greetText) != 0 {
		t.Fatalf("greeting sent into an existing dialog %#v", texts(after))
	}
}

// TestBusinessGreetingSkipsExistingDialog is the row 85 regression: a chat
// that already has a message does not get a no_activity_days=0 greeting the
// first time this server records the business setting.
func TestBusinessGreetingSkipsExistingDialog(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	biz, _ := dialPhone(t, ctx, cfg, "9996610437", "BizShop3")
	customer, _ := dialPhone(t, ctx, cfg, "9996610438", "BizGuest3")
	bizID, bizHash, err := customer.ImportPhone(ctx, "9996610437", "BizShop3")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerUser(bizID, bizHash)
	stamp := time.Now().UnixNano()
	if _, err = customer.SendText(ctx, peer, fmt.Sprintf("already-talking-%d", stamp), nil); err != nil {
		t.Fatal(err)
	}
	greetText := fmt.Sprintf("should-not-greet-%d", stamp)
	awayText := fmt.Sprintf("still-away-%d", stamp)
	greetID := saveShortcut(t, ctx, biz, "greet2", greetText)
	awayID := saveShortcut(t, ctx, biz, "away2", awayText)
	recipients := mtproto.MakeTLInputBusinessRecipients(&mtproto.InputBusinessRecipients{
		NonContacts: true,
	}).To_InputBusinessRecipients()
	if _, err = biz.Invoke(ctx, &mtproto.TLAccountUpdateBusinessGreetingMessage{
		Constructor: mtproto.CRC32_account_updateBusinessGreetingMessage,
		Message: mtproto.MakeTLInputBusinessGreetingMessage(&mtproto.InputBusinessGreetingMessage{
			ShortcutId:     greetID,
			NoActivityDays: 0,
			Recipients:     recipients,
		}).To_InputBusinessGreetingMessage(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = biz.Invoke(ctx, &mtproto.TLAccountUpdateBusinessAwayMessage{
		Constructor: mtproto.CRC32_account_updateBusinessAwayMessage,
		Message: mtproto.MakeTLInputBusinessAwayMessage(&mtproto.InputBusinessAwayMessage{
			ShortcutId: awayID,
			Schedule:   mtproto.MakeTLBusinessAwayMessageScheduleAlways(nil).To_BusinessAwayMessageSchedule(),
			Recipients: recipients,
		}).To_InputBusinessAwayMessage(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = customer.SendText(ctx, peer, fmt.Sprintf("talking-again-%d", stamp), nil); err != nil {
		t.Fatal(err)
	}
	msgs := waitHistory(t, ctx, customer, peer, awayText)
	if countText(msgs, greetText) != 0 {
		t.Fatalf("greeting on existing dialog %#v", texts(msgs))
	}
	if countText(msgs, awayText) != 1 {
		t.Fatalf("away %#v", texts(msgs))
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

func hasPlainText(msgs []*mtproto.Message) bool {
	for _, m := range msgs {
		if m != nil && m.GetMessage() != "" && m.GetAction() == nil {
			return true
		}
	}
	return false
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
