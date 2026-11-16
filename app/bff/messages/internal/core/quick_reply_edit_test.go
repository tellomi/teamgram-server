package core

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// editLocalKey is a chatbox document key for this process. It is not a phone
// number and it is not a users row.
func editLocalKey(slot int) int64 {
	return 8_802_000_000_000 + int64(os.Getpid()%1_000_000)*100 + int64(slot)
}

type savedAuthorProbe struct {
	messageclient.MessageClient
	calls    int
	userID   int64
	senderID int64
}

func (p *savedAuthorProbe) MessageGetUserMessageList(_ context.Context, in *message.TLMessageGetUserMessageList) (*message.Vector_MessageBox, error) {
	p.calls++
	id := int32(1)
	if len(in.IdList) > 0 {
		id = in.IdList[0]
	}
	return &message.Vector_MessageBox{Datas: []*mtproto.MessageBox{{
		UserId:       p.userID,
		MessageId:    id,
		SenderUserId: p.senderID,
		Message:      &mtproto.Message{Id: id, Message: "saved-forward"},
	}}}, nil
}

func TestEditQuickReplySkipsForeignSavedAuthor(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Fatal("TEAMGRAM_MYSQL_DSN is set; refusing to touch a configured database")
	}
	restore := chatbox.SwapShared(nil)
	t.Cleanup(restore)
	ctx := context.Background()
	owner := editLocalKey(1)
	stranger := editLocalKey(2)
	other := editLocalKey(3)
	auth := editLocalKey(4)
	self := mtproto.MakePeerUtil(mtproto.PEER_SELF, owner)
	shortcut := mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{Shortcut: "qredit"}).To_InputQuickReplyShortcut()
	ups, err := chatbox.Shared().CaptureQuickReply(ctx, owner, self, shortcut, []*mtproto.Message{
		{Message: "qr-first", FromId: mtproto.MakePeerUser(stranger)},
		{Message: "qr-second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var msgID int32
	for _, u := range ups.GetUpdates() {
		if u.GetMessage_MESSAGE().GetMessage() == "qr-first" {
			msgID = u.GetMessage_MESSAGE().GetId()
		}
	}
	page, err := chatbox.Shared().QuickReplies(ctx, owner, 0)
	if err != nil || msgID != 1 || len(page.GetQuickReplies()) != 1 {
		t.Fatalf("capture id=%d %+v %v", msgID, page, err)
	}
	sid := page.GetQuickReplies()[0].GetShortcutId()

	probe := &savedAuthorProbe{userID: owner, senderID: stranger}
	rec := &quickSync{}
	c := &MessagesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: probe, SyncClient: rec}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: owner, PermAuthKeyId: auth},
	}
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	_, err = c.MessagesEditMessage(&mtproto.TLMessagesEditMessage{
		Peer:    peer,
		Id:      msgID,
		Message: wrapperspb.String("saved-edit"),
	})
	if status.Convert(err).Message() != "MESSAGE_AUTHOR_REQUIRED" || probe.calls != 1 {
		t.Fatalf("saved author err=%v calls=%d", err, probe.calls)
	}
	stored, err := chatbox.Shared().QuickReplyMessages(ctx, owner, sid, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range stored.GetMessages() {
		if m.GetId() == msgID && m.GetMessage() != "qr-first" {
			t.Fatalf("author error still edited the shortcut: %+v", m)
		}
	}

	edited, err := c.MessagesEditMessage(&mtproto.TLMessagesEditMessage{
		Peer:                 peer,
		Id:                   msgID,
		Message:              wrapperspb.String("qr-rewritten"),
		QuickReplyShortcutId: wrapperspb.Int32(sid),
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatalf("shortcut edit consulted saved messages %d times", probe.calls)
	}
	got := edited.GetUpdates()[0].GetMessage_MESSAGE()
	if got.GetMessage() != "qr-rewritten" || got.GetFromId().GetUserId() != stranger || got.GetQuickReplyShortcutId().GetValue() != sid {
		t.Fatalf("edit result %+v", got)
	}
	if len(rec.notMe) != 1 || rec.notMe[0].GetUserId() != owner || rec.notMe[0].GetPermAuthKeyId() != auth {
		t.Fatalf("push %#v", rec.notMe)
	}
	pushed := updateBy(rec.notMe[0].GetUpdates(), mtproto.Predicate_updateQuickReplyMessage)
	if pushed.GetMessage_MESSAGE().GetMessage() != "qr-rewritten" {
		t.Fatalf("push body %+v", pushed)
	}

	_, err = c.MessagesEditMessage(&mtproto.TLMessagesEditMessage{
		Peer:                 peer,
		Id:                   msgID,
		Message:              wrapperspb.String("qr-rewritten"),
		QuickReplyShortcutId: wrapperspb.Int32(sid),
	})
	if status.Convert(err).Message() != "MESSAGE_NOT_MODIFIED" || len(rec.notMe) != 1 {
		t.Fatalf("duplicate err=%v pushes=%d", err, len(rec.notMe))
	}

	otherCore := &MessagesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: probe, SyncClient: rec}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: other, PermAuthKeyId: auth + 1},
	}
	_, err = otherCore.MessagesEditMessage(&mtproto.TLMessagesEditMessage{
		Peer:                 mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Id:                   msgID,
		Message:              wrapperspb.String("stolen"),
		QuickReplyShortcutId: wrapperspb.Int32(sid),
	})
	if status.Convert(err).Message() != "SHORTCUT_INVALID" {
		t.Fatalf("other account: %v", err)
	}
	left, err := chatbox.Shared().QuickReplyMessages(ctx, owner, sid, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	saw, sibling := false, false
	for _, m := range left.GetMessages() {
		if m.GetId() == msgID && m.GetMessage() == "qr-rewritten" {
			saw = true
		}
		if m.GetMessage() == "qr-second" {
			sibling = true
		}
	}
	if !saw || !sibling || probe.calls != 1 {
		t.Fatalf("after other account saw=%v sibling=%v calls=%d %+v", saw, sibling, probe.calls, left.GetMessages())
	}
}

func TestEditQuickReplyPushFailureKeepsTheEdit(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Fatal("TEAMGRAM_MYSQL_DSN is set; refusing to touch a configured database")
	}
	restore := chatbox.SwapShared(nil)
	t.Cleanup(restore)
	ctx := context.Background()
	owner := editLocalKey(11)
	auth := editLocalKey(12)
	ups, err := chatbox.Shared().CaptureQuickReply(ctx, owner, mtproto.MakePeerUtil(mtproto.PEER_SELF, owner), mtproto.MakeTLInputQuickReplyShortcut(&mtproto.InputQuickReplyShortcut{Shortcut: "pushfail"}).To_InputQuickReplyShortcut(), []*mtproto.Message{{Message: "kept"}})
	if err != nil {
		t.Fatal(err)
	}
	var msgID, sid int32
	for _, u := range ups.GetUpdates() {
		if u.GetMessage_MESSAGE().GetMessage() == "kept" {
			msgID = u.GetMessage_MESSAGE().GetId()
			sid = u.GetMessage_MESSAGE().GetQuickReplyShortcutId().GetValue()
		}
	}
	rec := &quickSync{err: errors.New("sync down")}
	c := &MessagesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{SyncClient: rec}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: owner, PermAuthKeyId: auth},
	}
	edited, err := c.MessagesEditMessage(&mtproto.TLMessagesEditMessage{
		Peer:                 mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Id:                   msgID,
		Message:              wrapperspb.String("kept-rewritten"),
		QuickReplyShortcutId: wrapperspb.Int32(sid),
	})
	if err != nil || edited.GetUpdates()[0].GetMessage_MESSAGE().GetMessage() != "kept-rewritten" {
		t.Fatalf("rpc %+v %v", edited, err)
	}
	page, err := chatbox.Shared().QuickReplyMessages(ctx, owner, sid, nil, 0)
	if err != nil || page.GetMessages()[0].GetMessage() != "kept-rewritten" {
		t.Fatalf("stored %+v %v", page, err)
	}
}

func TestEditQuickReplyShortcutFieldRoundTrip(t *testing.T) {
	in := &mtproto.TLMessagesEditMessage{
		Constructor:          mtproto.CRC32_messages_editMessage_b106e66c,
		Peer:                 mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Id:                   1,
		Message:              wrapperspb.String("qr-rewritten"),
		QuickReplyShortcutId: wrapperspb.Int32(7),
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := in.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	got, ok := mtproto.NewDecodeBuf(raw).Object().(*mtproto.TLMessagesEditMessage)
	if !ok || got.GetQuickReplyShortcutId().GetValue() != 7 || got.GetMessage().GetValue() != "qr-rewritten" || got.GetId() != 1 {
		t.Fatalf("layer 232 %#v", got)
	}
	old := &mtproto.TLMessagesEditMessage{
		Constructor:          mtproto.CRC32_messages_editMessage_48f71778,
		Peer:                 mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Id:                   1,
		Message:              wrapperspb.String("qr-rewritten"),
		QuickReplyShortcutId: wrapperspb.Int32(7),
	}
	buf = mtproto.NewEncodeBuf(256)
	if err := old.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	legacy, ok := mtproto.NewDecodeBuf(buf.GetBuf()).Object().(*mtproto.TLMessagesEditMessage)
	if !ok || legacy.GetQuickReplyShortcutId().GetValue() != 0 {
		t.Fatalf("old constructor kept shortcut id %#v", legacy)
	}
}
