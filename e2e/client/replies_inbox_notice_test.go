package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

const repliesUserID int64 = 1271266957

// TestRepliesInboxStaysClosedWithoutACommentThread runs against the default
// stack. TELLOMI_CHANNEL_COMMENTS is off, so there is no comment-thread index.
// An ordinary reply in the linked group, including one sent after the author
// leaves, must not become an @replies message. A real comment-thread notice
// is covered by the in-process tests that build the index; this stack does
// not turn the flag on.
func TestRepliesInboxStaysClosedWithoutACommentThread(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	commenter, _ := dialPhone(t, ctx, cfg, "9996610921", "ReplySrc")
	author, _ := dialPhone(t, ctx, cfg, "9996610922", "ReplyDst")
	authorID, authorHash, err := commenter.ImportPhone(ctx, "9996610922", "ReplyDst")
	if err != nil {
		t.Fatal(err)
	}
	newsID, newsHash := createBroadcast(t, ctx, commenter, "ReplyNews")
	groupID, groupHash, err := commenter.CreateMegagroup(ctx, "ReplyTalk")
	if err != nil {
		t.Fatal(err)
	}
	setDiscussion(t, ctx, commenter, inputChannel(newsID, newsHash), inputChannel(groupID, groupHash))
	if err = commenter.InviteToChannel(ctx, inputChannel(groupID, groupHash), inputUser(authorID, authorHash)); err != nil {
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
	parentID, err := author.SendText(ctx, inputPeerChannel(groupID, authorGroupHash), "parent-note", nil)
	if err != nil {
		t.Fatal(err)
	}
	pts, qts, date, err := author.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = commenter.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerChannel(groupID, groupHash),
		Message:     "ordinary-reply",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: parentID,
		}).To_InputReplyTo(),
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(800 * time.Millisecond)
	diff, err := author.Difference(ctx, pts, qts, date)
	if err != nil {
		t.Fatal(err)
	}
	if found := findRepliesNotice(append(allMessages(diff), messagesFromPushed(author.Pushed())...), "ordinary-reply"); found != nil {
		t.Fatalf("ordinary reply became a replies notice id %d", found.GetId())
	}
	if _, err = author.Invoke(ctx, &mtproto.TLChannelsLeaveChannel{
		Constructor: mtproto.CRC32_channels_leaveChannel,
		Channel:     inputChannel(groupID, authorGroupHash),
	}); err != nil {
		t.Fatal(err)
	}
	leftPts, leftQts, leftDate, err := author.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = commenter.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerChannel(groupID, groupHash),
		Message:     "after-leave",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: parentID,
		}).To_InputReplyTo(),
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(800 * time.Millisecond)
	leftDiff, err := author.Difference(ctx, leftPts, leftQts, leftDate)
	if err != nil {
		t.Fatal(err)
	}
	if found := findRepliesNotice(append(allMessages(leftDiff), messagesFromPushed(author.Pushed())...), "after-leave"); found != nil {
		t.Fatal("left member received the discussion reply")
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
		if peer == repliesUserID {
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
