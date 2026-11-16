package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestSendQuickReplyReachesOtherSessions(t *testing.T) {
	chatbox.ResetShared()
	const user int64 = 359201
	const auth int64 = 359211
	rec := &quickSync{}
	c := quickCore(user, auth, rec)
	self := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	shortcut := mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{Shortcut: "Hello"}).To_InputQuickReplyShortcut()

	ups, err := c.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
		Peer:               self,
		Message:            "hello there",
		RandomId:           3592011,
		QuickReplyShortcut: shortcut,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updateBy(ups, mtproto.Predicate_updateNewQuickReply) == nil || updateBy(ups, mtproto.Predicate_updateQuickReplyMessage) == nil {
		t.Fatalf("caller result %+v", predsOf(ups))
	}
	if len(rec.notMe) != 1 || rec.notMe[0].GetUserId() != user || rec.notMe[0].GetPermAuthKeyId() != auth {
		t.Fatalf("push %#v", rec.notMe)
	}
	pushed := rec.notMe[0].GetUpdates()
	card := updateBy(pushed, mtproto.Predicate_updateNewQuickReply)
	msg := updateBy(pushed, mtproto.Predicate_updateQuickReplyMessage)
	if card == nil || card.GetQuickReply().GetShortcut() != "hello" || card.GetQuickReply().GetCount() != 1 {
		t.Fatalf("new shortcut %+v", card)
	}
	if msg == nil || msg.GetMessage_MESSAGE().GetMessage() != "hello there" {
		t.Fatalf("message %+v", msg)
	}

	again, err := c.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
		Peer:               self,
		Message:            "second line",
		RandomId:           3592012,
		QuickReplyShortcut: shortcut,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updateBy(again, mtproto.Predicate_updateQuickReplyMessage).GetMessage_MESSAGE().GetMessage() != "second line" {
		t.Fatalf("append result %+v", predsOf(again))
	}
	if len(rec.notMe) != 2 {
		t.Fatalf("appended pushes %d", len(rec.notMe))
	}
	if got := updateBy(rec.notMe[1].GetUpdates(), mtproto.Predicate_updateQuickReplyMessage); got.GetMessage_MESSAGE().GetMessage() != "second line" {
		t.Fatalf("append push %+v", got)
	}
	if updateBy(rec.notMe[1].GetUpdates(), mtproto.Predicate_updateNewQuickReply).GetQuickReply().GetCount() != 2 {
		t.Fatal("append did not refresh the shortcut card")
	}
}

func TestSendMediaQuickReplyReachesOtherSessions(t *testing.T) {
	chatbox.ResetShared()
	const user int64 = 359202
	rec := &quickSync{}
	c := quickCore(user, user+10, rec)
	ups, err := c.MessagesSendMedia(&mtproto.TLMessagesSendMedia{
		Peer:               mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Media:              mtproto.MakeTLInputMediaEmpty(nil).To_InputMedia(),
		Message:            "a caption",
		RandomId:           3592021,
		QuickReplyShortcut: mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{Shortcut: "media"}).To_InputQuickReplyShortcut(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updateBy(ups, mtproto.Predicate_updateNewQuickReply) == nil || len(rec.notMe) != 1 {
		t.Fatalf("media %+v pushes %d", predsOf(ups), len(rec.notMe))
	}
	got := updateBy(rec.notMe[0].GetUpdates(), mtproto.Predicate_updateQuickReplyMessage)
	if got == nil || got.GetMessage_MESSAGE().GetMessage() != "a caption" {
		t.Fatalf("caption push %+v", got)
	}
}

func TestQuickReplySendToSomeoneElseDoesNotPush(t *testing.T) {
	chatbox.ResetShared()
	rec := &quickSync{}
	c := quickCore(359203, 11, rec)
	_, err := c.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
		Peer:               mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer(),
		Message:            "nope",
		RandomId:           3592031,
		QuickReplyShortcut: mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{Shortcut: "nope"}).To_InputQuickReplyShortcut(),
	})
	if err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("peer: %v", err)
	}
	if len(rec.notMe) != 0 {
		t.Fatalf("pushed a rejected shortcut: %d", len(rec.notMe))
	}
}

func TestQuickReplyPushFailureStillReturnsTheUpdates(t *testing.T) {
	chatbox.ResetShared()
	rec := &quickSync{err: errors.New("sync down")}
	c := quickCore(359204, 21, rec)
	ups, err := c.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
		Peer:               mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Message:            "kept",
		RandomId:           3592041,
		QuickReplyShortcut: mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{Shortcut: "kept"}).To_InputQuickReplyShortcut(),
	})
	if err != nil || updateBy(ups, mtproto.Predicate_updateQuickReplyMessage) == nil {
		t.Fatalf("rpc %+v %v", ups, err)
	}
	if len(rec.notMe) != 1 {
		t.Fatal("sync was not called")
	}
	page, err := chatbox.Shared().QuickReplies(context.Background(), 359204, 0)
	if err != nil || len(page.GetQuickReplies()) != 1 {
		t.Fatalf("stored %+v %v", page, err)
	}
}

func TestQuickReplyWithoutSyncStillStores(t *testing.T) {
	chatbox.ResetShared()
	c := quickCore(359205, 31, nil)
	ups, err := c.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
		Peer:               mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Message:            "local",
		RandomId:           3592051,
		QuickReplyShortcut: mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{Shortcut: "local"}).To_InputQuickReplyShortcut(),
	})
	if err != nil || len(ups.GetUpdates()) != 2 {
		t.Fatalf("%+v %v", ups, err)
	}
}

func quickCore(user, auth int64, syncer sync_client.SyncClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{SyncClient: syncer}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: user, PermAuthKeyId: auth},
	}
}

type quickSync struct {
	sync_client.SyncClient
	notMe []*sync.TLSyncUpdatesNotMe
	err   error
}

func (s *quickSync) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.notMe = append(s.notMe, in)
	if s.err != nil {
		return nil, s.err
	}
	return mtproto.EmptyVoid, nil
}

func updateBy(ups *mtproto.Updates, pred string) *mtproto.Update {
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

func predsOf(ups *mtproto.Updates) []string {
	out := make([]string, 0, len(ups.GetUpdates()))
	for _, up := range ups.GetUpdates() {
		out = append(out, up.GetPredicateName())
	}
	return out
}
