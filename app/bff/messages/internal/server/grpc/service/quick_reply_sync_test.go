package service

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

func TestQuickReplyMutationsReachOtherSessions(t *testing.T) {
	chatbox.ResetShared()
	const user int64 = 359301
	push := &quickPush{}
	svc := QuickReplyService{Push: push}
	ctx := withUser(user)
	auth := user + 10

	alpha, alphaMsg := captureShortcut(t, user, "alpha", "one")
	captureShortcut(t, user, "beta", "two")

	ok, err := svc.MessagesEditQuickReplyShortcut(ctx, &mtproto.TLMessagesEditQuickReplyShortcut{
		ShortcutId: alpha,
		Shortcut:   "later",
	})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("rename %+v %v", ok, err)
	}
	renamed := lastQuick(t, push)
	if renamed.GetUserId() != user || renamed.GetPermAuthKeyId() != auth {
		t.Fatalf("rename target %+v", renamed)
	}
	names := shortcutNames(renamed.GetUpdates())
	if len(names) != 2 || names[0] != "later" || names[1] != "beta" {
		t.Fatalf("rename list %v", names)
	}
	if renamed.GetUpdates().GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateQuickReplies {
		t.Fatalf("rename predicate %s", renamed.GetUpdates().GetUpdates()[0].GetPredicateName())
	}

	before := len(push.calls)
	if _, err := svc.MessagesEditQuickReplyShortcut(ctx, &mtproto.TLMessagesEditQuickReplyShortcut{
		ShortcutId: alpha,
		Shortcut:   "bad name",
	}); err != mtproto.ErrShortcutInvalid {
		t.Fatalf("bad name: %v", err)
	}
	if len(push.calls) != before {
		t.Fatal("a rejected rename was pushed")
	}

	beta := shortcutIDByName(t, renamed.GetUpdates(), "beta")
	ok, err = svc.MessagesReorderQuickReplies(ctx, &mtproto.TLMessagesReorderQuickReplies{
		Order: []int32{beta, alpha},
	})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("reorder %+v %v", ok, err)
	}
	order := shortcutIDs(lastQuick(t, push).GetUpdates())
	if len(order) != 2 || order[0] != beta || order[1] != alpha {
		t.Fatalf("order %v want %d,%d", order, beta, alpha)
	}
	if _, err := svc.MessagesReorderQuickReplies(ctx, &mtproto.TLMessagesReorderQuickReplies{
		Order: []int32{alpha},
	}); err != mtproto.ErrShortcutInvalid {
		t.Fatalf("partial order: %v", err)
	}
	if len(push.calls) != before+1 {
		t.Fatalf("partial order pushed: %d", len(push.calls))
	}

	ups, err := svc.MessagesDeleteQuickReplyMessages(ctx, &mtproto.TLMessagesDeleteQuickReplyMessages{
		ShortcutId: alpha,
		Id:         []int32{alphaMsg},
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted := updateNamed(ups, mtproto.Predicate_updateDeleteQuickReplyMessages)
	if deleted == nil || deleted.GetShortcutId() != alpha || len(deleted.GetMessages()) != 1 || deleted.GetMessages()[0] != alphaMsg {
		t.Fatalf("caller delete %+v", deleted)
	}
	if updateNamed(ups, mtproto.Predicate_updateNewQuickReply) == nil {
		t.Fatal("caller lost the refreshed shortcut card")
	}
	pushedDelete := updateNamed(lastQuick(t, push).GetUpdates(), mtproto.Predicate_updateDeleteQuickReplyMessages)
	if pushedDelete == nil || pushedDelete.GetShortcutId() != alpha || pushedDelete.GetMessages()[0] != alphaMsg {
		t.Fatalf("pushed delete %+v", pushedDelete)
	}

	ok, err = svc.MessagesDeleteQuickReplyShortcut(ctx, &mtproto.TLMessagesDeleteQuickReplyShortcut{ShortcutId: alpha})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("delete shortcut %+v %v", ok, err)
	}
	gone := updateNamed(lastQuick(t, push).GetUpdates(), mtproto.Predicate_updateDeleteQuickReply)
	if gone == nil || gone.GetShortcutId() != alpha {
		t.Fatalf("pushed delete shortcut %+v", gone)
	}
	if _, err := svc.MessagesDeleteQuickReplyShortcut(ctx, &mtproto.TLMessagesDeleteQuickReplyShortcut{ShortcutId: alpha}); err != mtproto.ErrShortcutInvalid {
		t.Fatalf("second delete: %v", err)
	}
}

func TestQuickReplySyncFailureDoesNotFailTheRPC(t *testing.T) {
	chatbox.ResetShared()
	const user int64 = 359302
	id, _ := captureShortcut(t, user, "stay", "body")
	push := &quickPush{err: errors.New("sync down")}
	ok, err := (QuickReplyService{Push: push}).MessagesEditQuickReplyShortcut(withUser(user), &mtproto.TLMessagesEditQuickReplyShortcut{
		ShortcutId: id,
		Shortcut:   "stayed",
	})
	if err != nil || !mtproto.FromBool(ok) || len(push.calls) != 1 {
		t.Fatalf("ok %+v err %v calls %d", ok, err, len(push.calls))
	}
	page, err := chatbox.Shared().QuickReplies(context.Background(), user, 0)
	if err != nil || page.GetQuickReplies()[0].GetShortcut() != "stayed" {
		t.Fatalf("stored %+v %v", page, err)
	}
}

func TestQuickReplyWithoutPushStillMutates(t *testing.T) {
	chatbox.ResetShared()
	const user int64 = 359303
	id, _ := captureShortcut(t, user, "quiet", "body")
	ok, err := (QuickReplyService{}).MessagesDeleteQuickReplyShortcut(withUser(user), &mtproto.TLMessagesDeleteQuickReplyShortcut{ShortcutId: id})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("%+v %v", ok, err)
	}
	page, err := chatbox.Shared().QuickReplies(context.Background(), user, 0)
	if err != nil || len(page.GetQuickReplies()) != 0 {
		t.Fatalf("left behind %+v %v", page, err)
	}
}

func captureShortcut(t *testing.T, user int64, name, text string) (int32, int32) {
	t.Helper()
	ups, err := chatbox.Shared().CaptureQuickReply(
		context.Background(),
		user,
		mtproto.MakeUserPeerUtil(user),
		mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{Shortcut: name}).To_InputQuickReplyShortcut(),
		[]*mtproto.Message{mtproto.MakeTLMessage(&mtproto.Message{Message: text}).To_Message()},
	)
	if err != nil {
		t.Fatal(err)
	}
	card := updateNamed(ups, mtproto.Predicate_updateNewQuickReply)
	msg := updateNamed(ups, mtproto.Predicate_updateQuickReplyMessage)
	if card == nil || msg == nil || card.GetQuickReply().GetShortcutId() == 0 || msg.GetMessage_MESSAGE().GetId() == 0 {
		t.Fatalf("capture %+v", ups)
	}
	return card.GetQuickReply().GetShortcutId(), msg.GetMessage_MESSAGE().GetId()
}

type quickPush struct {
	calls []*sync.TLSyncUpdatesNotMe
	err   error
}

func (p *quickPush) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	p.calls = append(p.calls, in)
	if p.err != nil {
		return nil, p.err
	}
	return mtproto.EmptyVoid, nil
}

func lastQuick(t *testing.T, p *quickPush) *sync.TLSyncUpdatesNotMe {
	t.Helper()
	if len(p.calls) == 0 {
		t.Fatal("no push")
	}
	return p.calls[len(p.calls)-1]
}

func updateNamed(ups *mtproto.Updates, pred string) *mtproto.Update {
	if ups == nil {
		return nil
	}
	for _, up := range ups.GetUpdates() {
		if up.GetPredicateName() == pred {
			return up
		}
	}
	return nil
}

func shortcutNames(ups *mtproto.Updates) []string {
	up := updateNamed(ups, mtproto.Predicate_updateQuickReplies)
	if up == nil {
		return nil
	}
	out := make([]string, 0, len(up.GetQuickReplies()))
	for _, q := range up.GetQuickReplies() {
		out = append(out, q.GetShortcut())
	}
	return out
}

func shortcutIDs(ups *mtproto.Updates) []int32 {
	up := updateNamed(ups, mtproto.Predicate_updateQuickReplies)
	if up == nil {
		return nil
	}
	out := make([]int32, 0, len(up.GetQuickReplies()))
	for _, q := range up.GetQuickReplies() {
		out = append(out, q.GetShortcutId())
	}
	return out
}

func shortcutIDByName(t *testing.T, ups *mtproto.Updates, name string) int32 {
	t.Helper()
	for _, q := range updateNamed(ups, mtproto.Predicate_updateQuickReplies).GetQuickReplies() {
		if q.GetShortcut() == name {
			return q.GetShortcutId()
		}
	}
	t.Fatalf("missing %s", name)
	return 0
}
