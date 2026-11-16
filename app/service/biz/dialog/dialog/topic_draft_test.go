package dialog

import (
	"testing"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func topicReply(top, replyTo int32) *mtproto.InputReplyTo {
	return mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId: replyTo,
		TopMsgId:     wrapperspb.Int32(top),
	}).To_InputReplyTo()
}

func TestInputTopicID(t *testing.T) {
	if InputTopicID(nil) != 0 {
		t.Fatal("nil")
	}
	if InputTopicID(topicReply(15, 15)) != 15 {
		t.Fatal("topic")
	}
	plain := mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{ReplyToMsgId: 4}).To_InputReplyTo()
	if InputTopicID(plain) != 0 {
		t.Fatal("dialog reply is not a topic")
	}
}

func TestReplyCarriesDraftBody(t *testing.T) {
	if ReplyCarriesDraftBody(topicReply(15, 0)) || ReplyCarriesDraftBody(topicReply(15, 15)) {
		t.Fatal("topic address is not draft content")
	}
	if !ReplyCarriesDraftBody(topicReply(15, 40)) {
		t.Fatal("reply inside the topic is content")
	}
	quoted := topicReply(15, 15)
	quoted.QuoteText = wrapperspb.String("q")
	if !ReplyCarriesDraftBody(quoted) {
		t.Fatal("quote")
	}
	if !ReplyCarriesDraftBody(mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{ReplyToMsgId: 3}).To_InputReplyTo()) {
		t.Fatal("dialog reply")
	}
}

func TestDraftIsEmptyKeepsTopicTextAndDropsAddressOnly(t *testing.T) {
	text := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{
		Message: "topic",
		ReplyTo: topicReply(15, 15),
	}).To_DraftMessage()
	if DraftIsEmpty(text) || DraftTopicID(text) != 15 {
		t.Fatal("text topic draft")
	}
	only := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{ReplyTo: topicReply(15, 15)}).To_DraftMessage()
	if !DraftIsEmpty(only) {
		t.Fatal("address-only draft should clear")
	}
	empty := mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{ReplyTo: topicReply(9, 0)}).To_DraftMessage()
	if !DraftIsEmpty(empty) || VisibleTopicDraft(empty) != nil {
		t.Fatal("empty topic draft is not forumTopic.draft")
	}
	if VisibleTopicDraft(text) == nil {
		t.Fatal("text topic draft is visible")
	}
	dialogDraft := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{Message: "group"}).To_DraftMessage()
	if VisibleTopicDraft(dialogDraft) != nil {
		t.Fatal("dialog draft must not fill a topic")
	}
}

func TestDraftUpdateSetsTopMsgIDOnlyForTopics(t *testing.T) {
	peer := mtproto.MakePeerChannel(7)
	topic := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{
		Message: "a",
		ReplyTo: topicReply(15, 0),
	}).To_DraftMessage()
	upd := DraftUpdate(peer, topic)
	if upd.GetTopMsgId_FLAGINT32().GetValue() != 15 || upd.GetDraft().GetMessage() != "a" {
		t.Fatalf("topic update %+v", upd)
	}
	group := DraftUpdate(peer, mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{Message: "g"}).To_DraftMessage())
	if group.GetTopMsgId_FLAGINT32() != nil {
		t.Fatalf("dialog update carried top_msg_id %+v", group.GetTopMsgId_FLAGINT32())
	}
}

func TestEnsureTopicReplyStampsMissingTop(t *testing.T) {
	msg := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{Message: "hi"}).To_DraftMessage()
	EnsureTopicReply(msg, 21)
	if DraftTopicID(msg) != 21 {
		t.Fatalf("stamped %d", DraftTopicID(msg))
	}
	EnsureTopicReply(msg, 21)
	if msg.GetReplyTo().GetReplyToMsgId() != 0 {
		t.Fatal("stamp replaced an existing reply id")
	}
}

func TestTopicKeysStayDistinct(t *testing.T) {
	type key struct {
		user int64
		peer int64
		top  int32
	}
	seen := map[key]string{}
	rows := []struct {
		key  key
		text string
	}{
		{key{1, 9, 0}, "group"},
		{key{1, 9, 15}, "topic-a"},
		{key{1, 9, 16}, "topic-b"},
		{key{2, 9, 15}, "other-user"},
	}
	for _, row := range rows {
		if prev, ok := seen[row.key]; ok {
			t.Fatalf("key %+v collapsed %q and %q", row.key, prev, row.text)
		}
		seen[row.key] = row.text
	}
	if len(seen) != 4 {
		t.Fatalf("keys %d", len(seen))
	}
}
