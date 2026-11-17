package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

const repliesUserID int64 = 1271266957

// TestRepliesInboxNoticeFromDiscussion is the real stack for a linked
// discussion reply while TELLOMI_CHANNEL_COMMENTS stays off. The author of
// the parent message gets one @replies inbox message. A bystander does not.
// blockFromReplies uses that inbox id and blocks the commenter.
func TestRepliesInboxNoticeFromDiscussion(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	commenter, commenterSelf := dialPhone(t, ctx, cfg, "9996610921", "ReplySrc")
	author, authorSelf := dialPhone(t, ctx, cfg, "9996610922", "ReplyDst")
	bystander, _ := dialPhone(t, ctx, cfg, "9996610923", "ReplyOut")
	authorID, authorHash, err := commenter.ImportPhone(ctx, "9996610922", "ReplyDst")
	if err != nil {
		t.Fatal(err)
	}
	if authorID != authorSelf.ID {
		t.Fatalf("imported author %d self %d", authorID, authorSelf.ID)
	}
	outID, outHash, err := commenter.ImportPhone(ctx, "9996610923", "ReplyOut")
	if err != nil {
		t.Fatal(err)
	}
	newsID, newsHash := createBroadcast(t, ctx, commenter, "ReplyNews")
	groupID, groupHash, err := commenter.CreateMegagroup(ctx, "ReplyTalk")
	if err != nil {
		t.Fatal(err)
	}
	setDiscussion(t, ctx, commenter, inputChannel(newsID, newsHash), inputChannel(groupID, groupHash))
	if err = commenter.InviteToChannel(ctx, inputChannel(groupID, groupHash), inputUser(authorID, authorHash), inputUser(outID, outHash)); err != nil {
		t.Fatal(err)
	}
	var authorGroupHash int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hash, ok, err := author.ChatAccess(ctx, groupID)
		if err != nil || !ok || hash == 0 {
			return false, err
		}
		authorGroupHash = hash
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	var bystanderGroupHash int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hash, ok, err := bystander.ChatAccess(ctx, groupID)
		if err != nil || !ok || hash == 0 {
			return false, err
		}
		bystanderGroupHash = hash
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	parentID, err := author.SendText(ctx, inputPeerChannel(groupID, authorGroupHash), "parent-note", nil)
	if err != nil {
		t.Fatal(err)
	}
	pts, qts, date, err := author.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bPts, bQts, bDate, err := bystander.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	replyObj, err := commenter.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerChannel(groupID, groupHash),
		Message:     "reply-note",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: parentID,
		}).To_InputReplyTo(),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyID := messageID(replyObj)
	if replyID == 0 {
		t.Fatal("discussion reply has no id")
	}
	var notice *mtproto.Message
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		diff, err := author.Difference(ctx, pts, qts, date)
		if err != nil {
			return false, err
		}
		notice = findRepliesNotice(append(allMessages(diff), messagesFromPushed(author.Pushed())...), "reply-note")
		return notice != nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if notice.GetFwdFrom().GetFromId().GetUserId() != commenterSelf.ID {
		t.Fatalf("fwd %#v", notice.GetFwdFrom())
	}
	if notice.GetFwdFrom().GetSavedFromPeer().GetChannelId() != groupID || notice.GetFwdFrom().GetSavedFromMsgId().GetValue() != replyID {
		t.Fatalf("source %#v reply %d", notice.GetFwdFrom(), replyID)
	}
	// The TL decoder fills reply_to_msg_id_FLAGINT32. The plain int field stays 0.
	replyToID := notice.GetReplyTo().GetFixedReplyToMsgId()
	replyPeer := notice.GetReplyTo().GetReplyToPeerId().GetChannelId()
	if replyPeer != groupID || replyToID != parentID {
		t.Fatalf("reply_to peer %d msg %d parent %d group %d", replyPeer, replyToID, parentID, groupID)
	}
	if notice.GetId() != replyID {
		if _, err = author.Invoke(ctx, &mtproto.TLContactsBlockFromReplies{
			Constructor: mtproto.CRC32_contacts_blockFromReplies,
			MsgId:       replyID,
		}); err == nil || !strings.Contains(status.Convert(err).Message(), "MSG_ID_INVALID") {
			t.Fatalf("channel id treated as inbox id: %v", err)
		}
	}
	blocked, err := author.Invoke(ctx, &mtproto.TLContactsBlockFromReplies{
		Constructor: mtproto.CRC32_contacts_blockFromReplies,
		MsgId:       notice.GetId(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawAuthor bool
	for _, upd := range updatesOf(blocked) {
		if upd.GetPredicateName() == mtproto.Predicate_updatePeerBlocked && upd.GetPeerId().GetUserId() == commenterSelf.ID {
			sawAuthor = true
		}
		if upd.GetPeerId().GetChannelId() == groupID || upd.GetPeerId().GetChannelId() == newsID {
			t.Fatalf("blocked a channel %#v", upd)
		}
	}
	if !sawAuthor {
		t.Fatalf("block result %T", blocked)
	}
	bDiff, err := bystander.Difference(ctx, bPts, bQts, bDate)
	if err != nil {
		t.Fatal(err)
	}
	if findRepliesNotice(append(allMessages(bDiff), messagesFromPushed(bystander.Pushed())...), "reply-note") != nil {
		t.Fatal("bystander received the replies notice")
	}
	groupMsgs, err := bystander.History(ctx, inputPeerChannel(groupID, bystanderGroupHash), 20)
	if err != nil {
		t.Fatal(err)
	}
	if idOfText(groupMsgs, "reply-note") == 0 {
		t.Fatal("discussion comment missing")
	}
	afterPts, afterQts, afterDate, err := author.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = commenter.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerChannel(groupID, groupHash),
		Message:     "reply-again",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: parentID,
		}).To_InputReplyTo(),
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	again, err := author.Difference(ctx, afterPts, afterQts, afterDate)
	if err != nil {
		t.Fatal(err)
	}
	if findRepliesNotice(append(allMessages(again), messagesFromPushed(author.Pushed())...), "reply-again") != nil {
		t.Fatal("blocked author still received a replies notice")
	}
}

func findRepliesNotice(msgs []*mtproto.Message, text string) *mtproto.Message {
	for _, m := range msgs {
		if m == nil || m.GetMessage() != text {
			continue
		}
		peer := m.GetPeerId().GetUserId()
		if peer == 0 {
			peer = m.GetFromId().GetUserId()
		}
		if peer == repliesUserID && m.GetFwdFrom().GetFromId().GetUserId() != 0 {
			return m
		}
	}
	return nil
}

func messagesFromPushed(objs []mtproto.TLObject) []*mtproto.Message {
	var out []*mtproto.Message
	for _, obj := range objs {
		out = append(out, allMessages(obj)...)
		for _, upd := range updatesOf(obj) {
			if m := upd.GetMessage_MESSAGE(); m != nil {
				out = append(out, m)
			}
		}
	}
	return out
}
