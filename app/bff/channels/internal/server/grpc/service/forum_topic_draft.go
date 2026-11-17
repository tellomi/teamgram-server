package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// fillTopicDrafts sets forumTopic.draft from the caller's topic drafts.
// The dialog draft (top_msg_id 0) is left off every topic. An empty topic
// draft is omitted so the flag stays unset.
func fillTopicDrafts(ctx context.Context, dialogs dialogclient.DialogClient, channelID int64, topics []*mtproto.ForumTopic) {
	if dialogs == nil || channelID == 0 || len(topics) == 0 {
		return
	}
	self, _ := actor(ctx)
	if self == 0 {
		return
	}
	got, err := dialogs.DialogGetAllDrafts(ctx, &dialog.TLDialogGetAllDrafts{UserId: self})
	if err != nil || got == nil {
		return
	}
	applyTopicDrafts(channelID, topics, got.GetDatas())
}

func applyTopicDrafts(channelID int64, topics []*mtproto.ForumTopic, items []*dialog.PeerWithDraftMessage) {
	byTop := map[int32]*mtproto.DraftMessage{}
	for _, item := range items {
		if item == nil || item.GetPeer().GetChannelId() != channelID {
			continue
		}
		draft := dialog.VisibleTopicDraft(item.GetDraft())
		if draft == nil {
			continue
		}
		byTop[dialog.DraftTopicID(draft)] = draft
	}
	for _, topic := range topics {
		if topic == nil || topic.GetPredicateName() == mtproto.Predicate_forumTopicDeleted {
			continue
		}
		if draft, ok := byTop[topic.GetId()]; ok {
			topic.Draft = draft
		}
	}
}
