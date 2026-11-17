//go:build replies_comments_e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestRepliesInboxCommentThreadNotifiesMember runs only in the CI job that
// starts an isolated stack with TELLOMI_CHANNEL_COMMENTS=1. The default
// client regression job does not compile this file. Missing env is a
// failure: a skip would be a green run without the stack.
func TestRepliesInboxCommentThreadNotifiesMember(t *testing.T) {
	if os.Getenv("TELLOMI_E2E") != "1" || os.Getenv("TELLOMI_E2E_COMMENTS") != "1" {
		t.Fatal("TELLOMI_E2E=1 and TELLOMI_E2E_COMMENTS=1 are required; a skip is not a pass")
	}
	t.Log("RUN TestRepliesInboxCommentThreadNotifiesMember")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	replier, replierSelf := dialPhone(t, ctx, cfg, "9996610931", "CmtSrc")
	author, authorSelf := dialPhone(t, ctx, cfg, "9996610932", "CmtDst")
	bystander, bystanderSelf := dialPhone(t, ctx, cfg, "9996610933", "CmtSee")
	authorID, authorHash, err := replier.ImportPhone(ctx, "9996610932", "CmtDst")
	if err != nil {
		t.Fatal(err)
	}
	bystanderID, bystanderHash, err := replier.ImportPhone(ctx, "9996610933", "CmtSee")
	if err != nil {
		t.Fatal(err)
	}
	replierID, replierHash, err := author.ImportPhone(ctx, "9996610931", "CmtSrc")
	if err != nil {
		t.Fatal(err)
	}
	if authorID != authorSelf.ID || bystanderID != bystanderSelf.ID || replierID != replierSelf.ID {
		t.Fatalf("users replier %d/%d author %d/%d bystander %d/%d", replierSelf.ID, replierID, authorSelf.ID, authorID, bystanderSelf.ID, bystanderID)
	}

	newsID, newsHash := createBroadcast(t, ctx, replier, "CmtNews")
	groupID, groupHash, err := replier.CreateMegagroup(ctx, "CmtTalk")
	if err != nil {
		t.Fatal(err)
	}
	setDiscussion(t, ctx, replier, inputChannel(newsID, newsHash), inputChannel(groupID, groupHash))
	if err = replier.InviteToChannel(ctx, inputChannel(groupID, groupHash), inputUser(authorID, authorHash)); err != nil {
		t.Fatal(err)
	}
	if err = replier.InviteToChannel(ctx, inputChannel(groupID, groupHash), inputUser(bystanderID, bystanderHash)); err != nil {
		t.Fatal(err)
	}
	var authorGroupHash, bystanderGroupHash int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		var okA, okB bool
		var err error
		authorGroupHash, okA, err = author.ChatAccess(ctx, groupID)
		if err != nil || !okA || authorGroupHash == 0 {
			return false, err
		}
		bystanderGroupHash, okB, err = bystander.ChatAccess(ctx, groupID)
		if err != nil || !okB || bystanderGroupHash == 0 {
			return false, err
		}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}

	postID, err := replier.SendText(ctx, inputPeerChannel(newsID, newsHash), "thread-post", nil)
	if err != nil {
		t.Fatal(err)
	}
	var rootID int32
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := replier.History(ctx, inputPeerChannel(groupID, groupHash), 20)
		if err != nil {
			return false, err
		}
		rootID = discussionRootID(msgs, postID)
		return rootID != 0 && rootID != postID, nil
	}); err != nil || rootID == 0 {
		t.Fatalf("channel post %d was not indexed into group %d: %v", postID, groupID, err)
	}

	parentObj, err := author.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerChannel(groupID, authorGroupHash),
		Message:     "member-note",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: rootID,
		}).To_InputReplyTo(),
	})
	if err != nil {
		t.Fatal(err)
	}
	parentID := messageID(parentObj)
	if parentID == 0 || parentID == rootID || parentID == postID {
		t.Fatalf("parent comment id %d root %d post %d", parentID, rootID, postID)
	}
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := bystander.History(ctx, inputPeerChannel(groupID, bystanderGroupHash), 20)
		if err != nil {
			return false, err
		}
		return hasMessageText(msgs, "member-note"), nil
	}); err != nil {
		t.Fatal(err)
	}

	replyObj, err := replier.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerChannel(groupID, groupHash),
		Message:     "member-answer",
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
		t.Fatal("reply has no id")
	}
	var notice *mtproto.Message
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		got, err := repliesTexts(ctx, author, "member-answer")
		if err != nil || len(got) != 1 {
			notice = nil
			return false, err
		}
		notice = got[0]
		return true, nil
	}); err != nil || notice == nil {
		t.Fatalf("author replies notices: %v", err)
	}
	if notice.GetId() == 0 || notice.GetId() == replyID || notice.GetId() == parentID || notice.GetId() == rootID || notice.GetId() == postID {
		t.Fatalf("box id %d is a channel id (reply %d parent %d root %d post %d)", notice.GetId(), replyID, parentID, rootID, postID)
	}
	peer := notice.GetPeerId().GetUserId()
	if peer == 0 {
		peer = notice.GetFromId().GetUserId()
	}
	if peer != repliesUserID {
		t.Fatalf("box peer %d", peer)
	}
	if notice.GetFwdFrom().GetFromId().GetUserId() != replierSelf.ID {
		t.Fatalf("fwd_from %d", notice.GetFwdFrom().GetFromId().GetUserId())
	}
	replyTo := notice.GetReplyTo()
	if replyTo.GetReplyToPeerId().GetChannelId() != groupID || replyTo.GetFixedReplyToMsgId() != parentID {
		t.Fatalf("reply_to peer %d msg %d", replyTo.GetReplyToPeerId().GetChannelId(), replyTo.GetFixedReplyToMsgId())
	}
	bystanderNotes, err := repliesTexts(ctx, bystander, "member-answer")
	if err != nil || len(bystanderNotes) != 0 {
		t.Fatalf("bystander notices %d %v", len(bystanderNotes), err)
	}
	t.Logf("PASS notice box %d group %d parent %d replier %d", notice.GetId(), groupID, parentID, replierSelf.ID)

	if err = author.blockUser(ctx, Self{ID: replierID, AccessHash: replierHash}); err != nil {
		t.Fatal(err)
	}
	if _, err = replier.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerChannel(groupID, groupHash),
		Message:     "blocked-answer",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: parentID,
		}).To_InputReplyTo(),
	}); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := author.History(ctx, inputPeerChannel(groupID, authorGroupHash), 20)
		if err != nil {
			return false, err
		}
		return hasMessageText(msgs, "blocked-answer"), nil
	}); err != nil {
		t.Fatal(err)
	}
	blocked, err := repliesTexts(ctx, author, "blocked-answer")
	if err != nil || len(blocked) != 0 {
		t.Fatalf("blocked reply wrote %d notices %v", len(blocked), err)
	}
	kept, err := repliesTexts(ctx, author, "member-answer")
	if err != nil || len(kept) != 1 || kept[0].GetId() != notice.GetId() {
		t.Fatalf("original notice changed %v %v", kept, err)
	}
	t.Log("PASS TestRepliesInboxCommentThreadNotifiesMember")
}

func discussionRootID(msgs []*mtproto.Message, postID int32) int32 {
	for _, m := range msgs {
		fwd := m.GetFwdFrom()
		if fwd.GetChannelPost().GetValue() == postID || fwd.GetSavedFromMsgId().GetValue() == postID {
			if m.GetId() != 0 && m.GetId() != postID {
				return m.GetId()
			}
		}
	}
	return 0
}

func hasMessageText(msgs []*mtproto.Message, text string) bool {
	for _, m := range msgs {
		if m.GetMessage() == text {
			return true
		}
	}
	return false
}

func repliesTexts(ctx context.Context, c *Client, text string) ([]*mtproto.Message, error) {
	msgs, err := c.History(ctx, inputPeerUser(repliesUserID, 0), 50)
	if err != nil {
		return nil, err
	}
	var out []*mtproto.Message
	for _, m := range msgs {
		if m.GetMessage() == text {
			out = append(out, m)
		}
	}
	return out, nil
}
