package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestForumTopicDrafts keeps a forum topic draft apart from the megagroup
// draft. Phone 9996610661, code 11111, two sessions. The other session's
// updateDraftMessage carries top_msg_id. getForumTopics and
// getForumTopicsByID return that topic's draft. Saving the group draft does
// not replace it. Clearing one topic leaves the other and the group draft.
func TestForumTopicDrafts(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()

	const phone = "9996610661"
	android := dialDraftSession(t, ctx, cfg, ProfileAndroid, phone, "DraftA")
	ios := dialDraftSession(t, ctx, cfg, ProfileIOS, phone, "DraftA")
	if _, _, _, err := ios.State(ctx); err != nil {
		t.Fatal(err)
	}

	created := invokeOK(t, android, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Megagroup:   true,
		Forum:       true,
		Title:       "DraftTopics",
		About:       "",
	})
	var chID, chHash int64
	for _, chat := range chatsOf(created) {
		if chat.GetId() != 0 {
			chID, chHash = chat.GetId(), chatAccessHash(chat)
		}
	}
	if chID == 0 {
		t.Fatalf("create channel %T", created)
	}
	peer := inputPeerChannel(chID, chHash)
	topicA := forumTopicID(t, invokeOK(t, android, &mtproto.TLMessagesCreateForumTopic{
		Constructor: mtproto.CRC32_messages_createForumTopic,
		Peer:        peer,
		Title:       "Alpha",
		RandomId:    randomID(),
	}))
	topicB := forumTopicID(t, invokeOK(t, android, &mtproto.TLMessagesCreateForumTopic{
		Constructor: mtproto.CRC32_messages_createForumTopic,
		Peer:        peer,
		Title:       "Beta",
		RandomId:    randomID(),
	}))
	if topicA == 0 || topicB == 0 || topicA == topicB {
		t.Fatalf("topics %d %d", topicA, topicB)
	}

	saveTopicDraft(t, android, peer, topicA, "alpha draft")
	if err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		return draftPushed(ios.Pushed(), chID, topicA, "alpha draft"), nil
	}); err != nil {
		t.Fatalf("other session did not receive topic draft: %v; pushed=%s", err, pushedSummary(ios.Pushed()))
	}

	saveTopicDraft(t, android, peer, topicB, "beta draft")
	saveDialogDraft(t, android, peer, "group draft")

	got := channelDrafts(t, android, peer)
	if got[0] != "group draft" || got[topicA] != "alpha draft" || got[topicB] != "beta draft" {
		t.Fatalf("getAllDrafts = %#v", got)
	}
	if dialogDraftText(t, android, chID) != "group draft" {
		t.Fatalf("dialog.draft = %q", dialogDraftText(t, android, chID))
	}
	topics := forumTopicDrafts(t, android, peer, 0)
	if topics[topicA] != "alpha draft" || topics[topicB] != "beta draft" {
		t.Fatalf("getForumTopics = %#v", topics)
	}
	byID := forumTopicDrafts(t, android, peer, topicA)
	if byID[topicA] != "alpha draft" {
		t.Fatalf("getForumTopicsByID = %#v", byID)
	}
	if _, ok := byID[topicB]; ok {
		t.Fatalf("getForumTopicsByID returned the other topic: %#v", byID)
	}

	saveTopicDraft(t, android, peer, topicA, "")
	got = channelDrafts(t, android, peer)
	if got[topicA] != "" || got[topicB] != "beta draft" || got[0] != "group draft" {
		t.Fatalf("after clearing topic A: %#v", got)
	}
	topics = forumTopicDrafts(t, android, peer, 0)
	if _, ok := topics[topicA]; ok || topics[topicB] != "beta draft" {
		t.Fatalf("forum topics after clearing A: %#v", topics)
	}

	invokeOK(t, android, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		ClearDraft:  true,
		Peer:        peer,
		Message:     "sent in beta",
		RandomId:    randomID(),
		ReplyTo:     topicReply(topicB),
	})
	if err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		left := channelDrafts(t, android, peer)
		return left[topicB] == "" && left[0] == "group draft", nil
	}); err != nil {
		t.Fatalf("clear_draft in topic B: %v; drafts=%#v", err, channelDrafts(t, android, peer))
	}
}

func dialDraftSession(t *testing.T, ctx context.Context, cfg Config, profile Profile, number, name string) *Client {
	t.Helper()
	c, err := Dial(ctx, cfg, profile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	if _, err := c.Login(ctx, Phone{Number: number, Code: "11111"}, name); err != nil {
		t.Fatal(err)
	}
	return c
}

func topicReply(top int32) *mtproto.InputReplyTo {
	return mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId: top,
		TopMsgId:     wrapperspb.Int32(top),
	}).To_InputReplyTo()
}

func saveTopicDraft(t *testing.T, c *Client, peer *mtproto.InputPeer, top int32, text string) {
	t.Helper()
	req := &mtproto.TLMessagesSaveDraft{
		Constructor: mtproto.CRC32_messages_saveDraft_ad0fa15c,
		Peer:        peer,
		Message:     text,
		ReplyTo:     topicReply(top),
	}
	invokeOK(t, c, req)
}

func saveDialogDraft(t *testing.T, c *Client, peer *mtproto.InputPeer, text string) {
	t.Helper()
	invokeOK(t, c, &mtproto.TLMessagesSaveDraft{
		Constructor: mtproto.CRC32_messages_saveDraft_ad0fa15c,
		Peer:        peer,
		Message:     text,
	})
}

func forumTopicID(t *testing.T, obj mtproto.TLObject) int32 {
	t.Helper()
	for _, upd := range updatesOf(obj) {
		msg := upd.GetMessage_MESSAGE()
		if msg == nil || msg.GetId() == 0 {
			continue
		}
		if msg.GetAction().GetPredicateName() == mtproto.Predicate_messageActionTopicCreate {
			return msg.GetId()
		}
	}
	t.Fatalf("createForumTopic returned %T without a topic", obj)
	return 0
}

func channelDrafts(t *testing.T, c *Client, peer *mtproto.InputPeer) map[int32]string {
	t.Helper()
	obj := invokeOK(t, c, &mtproto.TLMessagesGetAllDrafts{
		Constructor: mtproto.CRC32_messages_getAllDrafts,
	})
	out := map[int32]string{}
	chID := peer.GetChannelId()
	for _, upd := range updatesOf(obj) {
		if upd.GetPredicateName() != mtproto.Predicate_updateDraftMessage {
			continue
		}
		if upd.GetPeer_PEER().GetChannelId() != chID {
			continue
		}
		top := upd.GetTopMsgId_FLAGINT32().GetValue()
		text := ""
		if upd.GetDraft().GetPredicateName() != mtproto.Predicate_draftMessageEmpty {
			text = upd.GetDraft().GetMessage()
		}
		out[top] = text
	}
	return out
}

func draftPushed(objs []mtproto.TLObject, channelID int64, top int32, text string) bool {
	for _, obj := range objs {
		for _, upd := range updatesOf(obj) {
			if upd.GetPredicateName() != mtproto.Predicate_updateDraftMessage {
				continue
			}
			if upd.GetPeer_PEER().GetChannelId() == channelID &&
				upd.GetTopMsgId_FLAGINT32().GetValue() == top &&
				upd.GetDraft().GetMessage() == text {
				return true
			}
		}
	}
	return false
}

func dialogDraftText(t *testing.T, c *Client, channelID int64) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetDialogs{
		Constructor: mtproto.CRC32_messages_getDialogs,
		OffsetPeer:  inputPeerEmpty(),
		Limit:       100,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dialogsOf(obj) {
		if d.GetPeer().GetChannelId() != channelID {
			continue
		}
		if d.GetDraft().GetPredicateName() == mtproto.Predicate_draftMessageEmpty {
			return ""
		}
		return d.GetDraft().GetMessage()
	}
	t.Fatalf("channel %d missing from getDialogs", channelID)
	return ""
}

func forumTopicDrafts(t *testing.T, c *Client, peer *mtproto.InputPeer, only int32) map[int32]string {
	t.Helper()
	var obj mtproto.TLObject
	if only == 0 {
		obj = invokeOK(t, c, &mtproto.TLMessagesGetForumTopics{
			Constructor: mtproto.CRC32_messages_getForumTopics,
			Peer:        peer,
			Limit:       100,
		})
	} else {
		obj = invokeOK(t, c, &mtproto.TLMessagesGetForumTopicsByID{
			Constructor: mtproto.CRC32_messages_getForumTopicsByID,
			Peer:        peer,
			Topics:      []int32{only},
		})
	}
	out := map[int32]string{}
	for _, topic := range forumTopicsOf(t, obj) {
		if topic.GetDraft() == nil || topic.GetDraft().GetPredicateName() == mtproto.Predicate_draftMessageEmpty {
			continue
		}
		out[topic.GetId()] = topic.GetDraft().GetMessage()
	}
	return out
}

func forumTopicsOf(t *testing.T, obj mtproto.TLObject) []*mtproto.ForumTopic {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesForumTopics:
		if v.GetData2() != nil {
			return v.GetData2().GetTopics()
		}
	case *mtproto.Messages_ForumTopics:
		return v.GetTopics()
	default:
		t.Fatalf("forum topics %T", obj)
	}
	return nil
}
