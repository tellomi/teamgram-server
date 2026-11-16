package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDraftHasContent(t *testing.T) {
	if draftHasContent(nil) || draftHasContent(&mtproto.TLMessagesSaveDraft{}) {
		t.Fatal("empty draft should clear")
	}
	if !draftHasContent(&mtproto.TLMessagesSaveDraft{Message: "hi"}) {
		t.Fatal("text")
	}
	if !draftHasContent(&mtproto.TLMessagesSaveDraft{InvertMedia: true}) {
		t.Fatal("invert_media")
	}
	if !draftHasContent(&mtproto.TLMessagesSaveDraft{Media: mtproto.MakeTLInputMediaEmpty(&mtproto.InputMedia{}).To_InputMedia()}) {
		t.Fatal("media")
	}
	if !draftHasContent(&mtproto.TLMessagesSaveDraft{Effect: &wrapperspb.Int64Value{Value: 7}}) {
		t.Fatal("effect")
	}
	if !draftHasContent(&mtproto.TLMessagesSaveDraft{SuggestedPost: &mtproto.SuggestedPost{}}) {
		t.Fatal("suggested_post")
	}
	if !draftHasContent(&mtproto.TLMessagesSaveDraft{NoWebpage: true}) {
		t.Fatal("no_webpage")
	}
	address := mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId: 15,
		TopMsgId:     wrapperspb.Int32(15),
	}).To_InputReplyTo()
	if draftHasContent(&mtproto.TLMessagesSaveDraft{ReplyTo: address}) {
		t.Fatal("topic address alone should clear that topic")
	}
	if !draftHasContent(&mtproto.TLMessagesSaveDraft{ReplyTo: address, Message: "inside"}) {
		t.Fatal("topic text")
	}
	if !draftHasContent(&mtproto.TLMessagesSaveDraft{ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId: 40,
		TopMsgId:     wrapperspb.Int32(15),
	}).To_InputReplyTo()}) {
		t.Fatal("reply inside a topic")
	}
	if saveDraftTopicID(&mtproto.TLMessagesSaveDraft{TopMsgId: wrapperspb.Int32(8)}) != 8 {
		t.Fatal("request top_msg_id")
	}
	if saveDraftTopicID(&mtproto.TLMessagesSaveDraft{ReplyTo: address}) != 15 {
		t.Fatal("reply top_msg_id")
	}
}
