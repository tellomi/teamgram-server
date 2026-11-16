package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestOutgoingTopicID(t *testing.T) {
	if outgoingTopicID(nil, nil) != 0 {
		t.Fatal("dialog send")
	}
	reply := mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId: 40,
		TopMsgId:     wrapperspb.Int32(15),
	}).To_InputReplyTo()
	if outgoingTopicID(reply, nil) != 15 {
		t.Fatal("reply_to.top_msg_id")
	}
	if outgoingTopicID(nil, wrapperspb.Int32(8)) != 8 {
		t.Fatal("request top_msg_id")
	}
	if outgoingTopicID(reply, wrapperspb.Int32(8)) != 8 {
		t.Fatal("request flag wins")
	}
}
