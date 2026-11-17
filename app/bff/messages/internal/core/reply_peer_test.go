package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestReplyToAnotherChatKeepsTheQuote(t *testing.T) {
	in := mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId:  15,
		ReplyToPeerId: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer(),
		QuoteText:     wrapperspb.String("hello"),
		QuoteOffset:   wrapperspb.Int32(2),
	}).To_InputReplyTo()
	h := replyHeaderFromInput(3, mtproto.MakePeerUtil(mtproto.PEER_USER, 4), in)
	if h == nil {
		t.Fatal("header dropped")
	}
	if h.GetQuoteText().GetValue() != "hello" || !h.GetQuote() {
		t.Fatalf("quote %+v", h)
	}
	if h.GetReplyToPeerId().GetUserId() != 9 || h.GetReplyToMsgId() != 15 {
		t.Fatalf("peer/id %+v", h)
	}
	if h.GetQuoteOffset().GetValue() != 2 {
		t.Fatalf("offset %+v", h.GetQuoteOffset())
	}
	same := replyHeaderFromInput(3, mtproto.MakePeerUtil(mtproto.PEER_USER, 9), in)
	if same.GetReplyToPeerId() != nil {
		t.Fatal("reply_to_peer_id names this dialog and should be cleared")
	}
	if same.GetQuoteText().GetValue() != "hello" || same.GetReplyToMsgId() != 15 {
		t.Fatalf("same-dialog quote %+v", same)
	}
	if same.GetTodoItemId().GetValue() != 0 {
		t.Fatal("todo id was invented")
	}
	withItem := mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId: 15,
		TodoItemId:   wrapperspb.Int32(3),
	}).To_InputReplyTo()
	itemHeader := replyHeaderFromInput(3, mtproto.MakePeerUtil(mtproto.PEER_USER, 9), withItem)
	if itemHeader.GetTodoItemId().GetValue() != 3 {
		t.Fatalf("todo item id dropped %+v", itemHeader)
	}
}

func TestDropAuthorClearsFwdFrom(t *testing.T) {
	m := &mtproto.Message{
		Message: "x",
		FwdFrom: mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
			FromId: mtproto.MakePeerUser(4),
			Date:   10,
		}).To_MessageFwdHeader(),
		SavedPeerId: mtproto.MakePeerUser(4),
		PostAuthor:  wrapperspb.String("Ada"),
	}
	dropForwardAuthor(m)
	if m.GetFwdFrom() != nil || m.GetSavedPeerId() != nil || m.GetPostAuthor() != nil {
		t.Fatalf("still forwarded %+v", m)
	}
	if m.GetMessage() != "x" {
		t.Fatal("text dropped")
	}
}

func TestDropMediaCaptionsKeepsTheFileAndPlainText(t *testing.T) {
	photo := &mtproto.Message{
		Message: "look",
		Entities: []*mtproto.MessageEntity{
			mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{Offset: 0, Length: 4}).To_MessageEntity(),
		},
		Media: mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
			Photo_FLAGPHOTO: mtproto.MakeTLPhoto(&mtproto.Photo{Id: 7}).To_Photo(),
		}).To_MessageMedia(),
		FwdFrom: mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
			FromId: mtproto.MakePeerUser(4),
			Date:   10,
		}).To_MessageFwdHeader(),
	}
	dropMediaCaptions(photo)
	if photo.GetMessage() != "" || len(photo.GetEntities()) != 0 {
		t.Fatalf("caption left %+v", photo.GetMessage())
	}
	if photo.GetMedia().GetPhoto_FLAGPHOTO().GetId() != 7 {
		t.Fatal("photo removed")
	}
	if photo.GetFwdFrom().GetFromId().GetUserId() != 4 {
		t.Fatal("drop_media_captions also hid the author")
	}

	doc := &mtproto.Message{
		Message: "clip",
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: mtproto.MakeTLDocument(&mtproto.Document{Id: 9}).To_Document(),
		}).To_MessageMedia(),
	}
	dropForwardAuthor(doc)
	dropMediaCaptions(doc)
	if doc.GetMessage() != "" || doc.GetFwdFrom() != nil || doc.GetMedia().GetDocument().GetId() != 9 {
		t.Fatalf("document %+v", doc)
	}

	text := &mtproto.Message{Message: "hello"}
	dropMediaCaptions(text)
	if text.GetMessage() != "hello" {
		t.Fatal("plain text stripped")
	}
	page := &mtproto.Message{
		Message: "https://example.com",
		Media:   mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{}).To_MessageMedia(),
	}
	dropMediaCaptions(page)
	if page.GetMessage() != "https://example.com" {
		t.Fatal("link preview text stripped")
	}
}
