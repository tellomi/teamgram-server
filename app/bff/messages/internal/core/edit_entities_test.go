package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestApplyEditEntitiesKeepsBold(t *testing.T) {
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Message: "hello",
		Entities: []*mtproto.MessageEntity{
			mtproto.MakeTLMessageEntityItalic(&mtproto.MessageEntity{Offset: 0, Length: 5}).To_MessageEntity(),
		},
	}).To_Message()
	bold := []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{Offset: 0, Length: 5}).To_MessageEntity(),
	}
	applyEditEntities(msg, &wrapperspb.StringValue{Value: "hello"}, bold)
	if msg.GetMessage() != "hello" {
		t.Fatalf("text = %q", msg.GetMessage())
	}
	if len(msg.GetEntities()) != 1 || msg.GetEntities()[0].GetPredicateName() != mtproto.Predicate_messageEntityBold {
		t.Fatalf("entities = %#v", msg.GetEntities())
	}

	applyEditEntities(msg, &wrapperspb.StringValue{Value: "plain"}, nil)
	if msg.GetMessage() != "plain" || msg.GetEntities() != nil {
		t.Fatalf("text-only edit left entities %#v on %q", msg.GetEntities(), msg.GetMessage())
	}

	only := []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{Offset: 0, Length: 5}).To_MessageEntity(),
	}
	applyEditEntities(msg, nil, only)
	if msg.GetMessage() != "plain" || len(msg.GetEntities()) != 1 {
		t.Fatalf("entity-only edit = %q %#v", msg.GetMessage(), msg.GetEntities())
	}
}
