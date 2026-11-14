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
	h := replyHeaderFromInput(3, in)
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
