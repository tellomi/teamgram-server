package service

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func topicDraftItem(channelID int64, top int32, text string, empty bool) *dialog.PeerWithDraftMessage {
	var draft *mtproto.DraftMessage
	if empty {
		draft = mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{}).To_DraftMessage()
	} else {
		draft = mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{Message: text}).To_DraftMessage()
	}
	dialog.EnsureTopicReply(draft, top)
	return dialog.MakeTLUpdateDraftMessage(&dialog.PeerWithDraftMessage{
		Peer:  mtproto.MakePeerChannel(channelID),
		Draft: draft,
	}).To_PeerWithDraftMessage()
}

func TestApplyTopicDraftsDoesNotMixTopicsOrTheDialog(t *testing.T) {
	const channelID int64 = 9
	topics := []*mtproto.ForumTopic{
		mtproto.MakeTLForumTopic(&mtproto.ForumTopic{Id: 15, Title: "A"}).To_ForumTopic(),
		mtproto.MakeTLForumTopic(&mtproto.ForumTopic{Id: 16, Title: "B"}).To_ForumTopic(),
		mtproto.MakeTLForumTopicDeleted(&mtproto.ForumTopic{Id: 17}).To_ForumTopic(),
	}
	group := dialog.MakeTLUpdateDraftMessage(&dialog.PeerWithDraftMessage{
		Peer:  mtproto.MakePeerChannel(channelID),
		Draft: mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{Message: "group"}).To_DraftMessage(),
	}).To_PeerWithDraftMessage()
	other := topicDraftItem(99, 15, "other channel", false)
	applyTopicDrafts(channelID, topics, []*dialog.PeerWithDraftMessage{
		topicDraftItem(channelID, 15, "alpha", false),
		topicDraftItem(channelID, 16, "", true),
		group,
		other,
	})
	if topics[0].GetDraft().GetMessage() != "alpha" {
		t.Fatalf("topic 15 %+v", topics[0].GetDraft())
	}
	if topics[1].GetDraft() != nil {
		t.Fatalf("empty topic draft was attached: %+v", topics[1].GetDraft())
	}
	if topics[2].GetDraft() != nil {
		t.Fatal("deleted topic gained a draft")
	}
	if group.GetDraft().GetMessage() != "group" || group.GetDraft().GetReplyTo() != nil {
		t.Fatal("dialog draft was rewritten")
	}
}

func TestApplyTopicDraftsUsesStoredTop(t *testing.T) {
	draft := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{
		Message: "kept",
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: 40,
			TopMsgId:     wrapperspb.Int32(15),
		}).To_InputReplyTo(),
	}).To_DraftMessage()
	topic := mtproto.MakeTLForumTopic(&mtproto.ForumTopic{Id: 15}).To_ForumTopic()
	applyTopicDrafts(4, []*mtproto.ForumTopic{topic}, []*dialog.PeerWithDraftMessage{
		dialog.MakeTLUpdateDraftMessage(&dialog.PeerWithDraftMessage{
			Peer:  mtproto.MakePeerChannel(4),
			Draft: draft,
		}).To_PeerWithDraftMessage(),
	})
	if topic.GetDraft().GetMessage() != "kept" || topic.GetDraft().GetReplyTo().GetReplyToMsgId() != 40 {
		t.Fatalf("%+v", topic.GetDraft())
	}
}
