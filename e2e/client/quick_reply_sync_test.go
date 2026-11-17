package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestQuickReplyChangesReachTheOtherSession is ISSUE-S-QUICKREPLY-SYNC-001
// (问题清单第 359 行). Session 1 creates, appends, renames, reorders, deletes
// a message, and deletes a shortcut. Session 2 must see each update without
// calling getQuickReplies. Phone 9996610711, code 11111.
func TestQuickReplyChangesReachTheOtherSession(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	a, _ := dialPhone(t, ctx, cfg, "9996610711", "QuickA")
	b, _ := dialPhone(t, ctx, cfg, "9996610711", "QuickB")
	clearQuickReplies(t, a)
	if _, _, _, err := b.State(ctx); err != nil {
		t.Fatal(err)
	}

	created := invokeOK(t, a, quickSend("alpha", "alpha-one"))
	alpha, alphaOne := quickIDs(t, created)
	waitQuick(t, ctx, b, func(obj mtproto.TLObject) bool {
		return quickPushHas(obj, mtproto.Predicate_updateNewQuickReply, func(up *mtproto.Update) bool {
			return up.GetQuickReply().GetShortcut() == "alpha" && up.GetQuickReply().GetShortcutId() == alpha
		}) && quickPushHas(obj, mtproto.Predicate_updateQuickReplyMessage, func(up *mtproto.Update) bool {
			return up.GetMessage_MESSAGE().GetMessage() == "alpha-one" && up.GetMessage_MESSAGE().GetId() == alphaOne
		})
	})

	if _, _, _, err := b.State(ctx); err != nil {
		t.Fatal(err)
	}
	appended := invokeOK(t, a, quickSend("alpha", "alpha-two"))
	_, alphaTwo := quickIDs(t, appended)
	if alphaTwo == 0 || alphaTwo == alphaOne {
		t.Fatalf("append id %d", alphaTwo)
	}
	waitQuick(t, ctx, b, func(obj mtproto.TLObject) bool {
		return quickPushHas(obj, mtproto.Predicate_updateNewQuickReply, func(up *mtproto.Update) bool {
			return up.GetQuickReply().GetShortcutId() == alpha && up.GetQuickReply().GetCount() == 2
		}) && quickPushHas(obj, mtproto.Predicate_updateQuickReplyMessage, func(up *mtproto.Update) bool {
			return up.GetMessage_MESSAGE().GetMessage() == "alpha-two"
		})
	})

	if _, _, _, err := b.State(ctx); err != nil {
		t.Fatal(err)
	}
	second := invokeOK(t, a, quickSend("beta", "beta-one"))
	beta, _ := quickIDs(t, second)
	waitQuick(t, ctx, b, func(obj mtproto.TLObject) bool {
		return quickPushHas(obj, mtproto.Predicate_updateNewQuickReply, func(up *mtproto.Update) bool {
			return up.GetQuickReply().GetShortcut() == "beta" && up.GetQuickReply().GetShortcutId() == beta
		})
	})

	if _, _, _, err := b.State(ctx); err != nil {
		t.Fatal(err)
	}
	invokeOK(t, a, &mtproto.TLMessagesEditQuickReplyShortcut{
		Constructor: mtproto.CRC32_messages_editQuickReplyShortcut,
		ShortcutId:  alpha,
		Shortcut:    "later",
	})
	waitQuick(t, ctx, b, func(obj mtproto.TLObject) bool {
		return quickNames(obj, "later", "beta")
	})

	if _, _, _, err := b.State(ctx); err != nil {
		t.Fatal(err)
	}
	invokeOK(t, a, &mtproto.TLMessagesReorderQuickReplies{
		Constructor: mtproto.CRC32_messages_reorderQuickReplies,
		Order:       []int32{beta, alpha},
	})
	waitQuick(t, ctx, b, func(obj mtproto.TLObject) bool {
		ids := quickShortcutIDs(obj)
		return len(ids) == 2 && ids[0] == beta && ids[1] == alpha
	})

	if _, _, _, err := b.State(ctx); err != nil {
		t.Fatal(err)
	}
	deleted := invokeOK(t, a, &mtproto.TLMessagesDeleteQuickReplyMessages{
		Constructor: mtproto.CRC32_messages_deleteQuickReplyMessages,
		ShortcutId:  alpha,
		Id:          []int32{alphaTwo},
	})
	if !quickPushHas(deleted, mtproto.Predicate_updateDeleteQuickReplyMessages, func(up *mtproto.Update) bool {
		return up.GetShortcutId() == alpha && containsInt(up.GetMessages(), alphaTwo)
	}) {
		t.Fatalf("delete messages result did not include the update: %s", pushedSummary([]mtproto.TLObject{deleted}))
	}
	waitQuick(t, ctx, b, func(obj mtproto.TLObject) bool {
		return quickPushHas(obj, mtproto.Predicate_updateDeleteQuickReplyMessages, func(up *mtproto.Update) bool {
			return up.GetShortcutId() == alpha && containsInt(up.GetMessages(), alphaTwo)
		})
	})

	if _, _, _, err := b.State(ctx); err != nil {
		t.Fatal(err)
	}
	invokeOK(t, a, &mtproto.TLMessagesDeleteQuickReplyShortcut{
		Constructor: mtproto.CRC32_messages_deleteQuickReplyShortcut,
		ShortcutId:  alpha,
	})
	waitQuick(t, ctx, b, func(obj mtproto.TLObject) bool {
		return quickPushHas(obj, mtproto.Predicate_updateDeleteQuickReply, func(up *mtproto.Update) bool {
			return up.GetShortcutId() == alpha
		})
	})

	page := invokeOK(t, b, &mtproto.TLMessagesGetQuickReplies{Constructor: mtproto.CRC32_messages_getQuickReplies})
	replies, ok := quickReplyPage(page)
	if !ok || len(replies) != 1 || replies[0].GetShortcutId() != beta || replies[0].GetShortcut() != "beta" {
		t.Fatalf("session 2 list %+v", replies)
	}
}

func clearQuickReplies(t *testing.T, c *Client) {
	t.Helper()
	page := invokeOK(t, c, &mtproto.TLMessagesGetQuickReplies{Constructor: mtproto.CRC32_messages_getQuickReplies})
	replies, ok := quickReplyPage(page)
	if !ok {
		t.Fatalf("getQuickReplies %T", page)
	}
	for _, q := range replies {
		invokeOK(t, c, &mtproto.TLMessagesDeleteQuickReplyShortcut{
			Constructor: mtproto.CRC32_messages_deleteQuickReplyShortcut,
			ShortcutId:  q.GetShortcutId(),
		})
	}
}

func quickSend(name, text string) *mtproto.TLMessagesSendMessage {
	return &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerSelf(),
		Message:     text,
		RandomId:    randomID(),
		QuickReplyShortcut: mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{
			Shortcut: name,
		}).To_InputQuickReplyShortcut(),
	}
}

func waitQuick(t *testing.T, ctx context.Context, c *Client, match func(mtproto.TLObject) bool) {
	t.Helper()
	err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		for _, obj := range c.Pushed() {
			if match(obj) {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("other session missed the quick-reply update: %v; pushed=%s", err, pushedSummary(c.Pushed()))
	}
}

func quickPushHas(obj mtproto.TLObject, pred string, match func(*mtproto.Update) bool) bool {
	for _, up := range updatesOf(obj) {
		if up.GetPredicateName() == pred && match(up) {
			return true
		}
	}
	return false
}

func quickIDs(t *testing.T, obj mtproto.TLObject) (shortcutID, msgID int32) {
	t.Helper()
	for _, up := range updatesOf(obj) {
		if q := up.GetQuickReply(); q != nil && q.GetShortcutId() != 0 {
			shortcutID = q.GetShortcutId()
		}
		if m := up.GetMessage_MESSAGE(); m != nil && m.GetId() != 0 {
			msgID = m.GetId()
		}
	}
	if shortcutID == 0 || msgID == 0 {
		t.Fatalf("send did not return a shortcut and a message: %s", pushedSummary([]mtproto.TLObject{obj}))
	}
	return shortcutID, msgID
}

func quickNames(obj mtproto.TLObject, want ...string) bool {
	got := quickShortcutNames(obj)
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func quickShortcutNames(obj mtproto.TLObject) []string {
	for _, up := range updatesOf(obj) {
		if up.GetPredicateName() != mtproto.Predicate_updateQuickReplies {
			continue
		}
		out := make([]string, 0, len(up.GetQuickReplies()))
		for _, q := range up.GetQuickReplies() {
			out = append(out, q.GetShortcut())
		}
		return out
	}
	return nil
}

func quickShortcutIDs(obj mtproto.TLObject) []int32 {
	for _, up := range updatesOf(obj) {
		if up.GetPredicateName() != mtproto.Predicate_updateQuickReplies {
			continue
		}
		out := make([]int32, 0, len(up.GetQuickReplies()))
		for _, q := range up.GetQuickReplies() {
			out = append(out, q.GetShortcutId())
		}
		return out
	}
	return nil
}

func quickReplyPage(obj mtproto.TLObject) ([]*mtproto.QuickReply, bool) {
	switch v := obj.(type) {
	case *mtproto.Messages_QuickReplies:
		return v.GetQuickReplies(), v.GetPredicateName() == mtproto.Predicate_messages_quickReplies
	case *mtproto.TLMessagesQuickReplies:
		return v.To_Messages_QuickReplies().GetQuickReplies(), true
	default:
		return nil, false
	}
}

func containsInt(ids []int32, want int32) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
