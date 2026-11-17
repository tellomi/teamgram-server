package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestBlockFromRepliesSupergroupAndDiscussion is ISSUE-S-BLOCK-FROM-REPLIES-001.
// The caller passes the supergroup reply id, then a discussion-group comment
// id, with every flag false. CHANNEL_COMMENTS is not set by this test.
// Phones 9996610871 / 9996610872, code 11111, layer 232.
func TestBlockFromRepliesSupergroupAndDiscussion(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610871", "BfrA")
	b, _ := dialPhone(t, ctx, cfg, "9996610872", "BfrB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610872", "BfrB")
	if err != nil {
		t.Fatal(err)
	}
	bSelf := Self{ID: bID, AccessHash: bHash}

	megaID, megaHash, err := a.CreateMegagroup(ctx, "bfr-mega")
	if err != nil {
		t.Fatal(err)
	}
	mega := inputChannel(megaID, megaHash)
	defer deleteChannel(ctx, a, mega)
	if err = a.InviteToChannel(ctx, mega, inputUser(bSelf.ID, bSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	var bMega int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hash, ok, err := b.ChatAccess(ctx, megaID)
		if err != nil || !ok {
			return false, err
		}
		bMega = hash
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	rootID, err := a.SendText(ctx, inputPeerChannel(megaID, megaHash), "bfr-root", nil)
	if err != nil {
		t.Fatal(err)
	}
	replyObj, err := b.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerChannel(megaID, bMega),
		Message:     "bfr-reply",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: rootID,
		}).To_InputReplyTo(),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyID := messageID(replyObj)
	if replyID == 0 {
		t.Fatal("reply has no id")
	}
	defer func() { _ = a.unblockUser(ctx, bSelf) }()
	assertBlockFromReplies(t, ctx, a, bSelf.ID, replyID)
	if err = a.unblockUser(ctx, bSelf); err != nil {
		t.Fatal(err)
	}

	newsID, newsHash := createBroadcast(t, ctx, a, "bfr-news")
	defer deleteChannel(ctx, a, inputChannel(newsID, newsHash))
	talkID, talkHash, err := a.CreateMegagroup(ctx, "bfr-talk")
	if err != nil {
		t.Fatal(err)
	}
	talk := inputChannel(talkID, talkHash)
	defer deleteChannel(ctx, a, talk)
	setDiscussion(t, ctx, a, inputChannel(newsID, newsHash), talk)
	if err = a.InviteToChannel(ctx, talk, inputUser(bSelf.ID, bSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	var bTalk int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hash, ok, err := b.ChatAccess(ctx, talkID)
		if err != nil || !ok {
			return false, err
		}
		bTalk = hash
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.SendText(ctx, inputPeerChannel(newsID, newsHash), "bfr-post", nil); err != nil {
		t.Fatal(err)
	}
	commentID, err := b.SendText(ctx, inputPeerChannel(talkID, bTalk), "bfr-comment", nil)
	if err != nil {
		t.Fatal(err)
	}
	assertBlockFromReplies(t, ctx, a, bSelf.ID, commentID)

	if _, err = a.Invoke(ctx, &mtproto.TLContactsBlockFromReplies{
		Constructor: mtproto.CRC32_contacts_blockFromReplies,
		MsgId:       2_000_000_000,
	}); err == nil || !strings.Contains(err.Error(), "MSG_ID_INVALID") {
		t.Fatalf("unknown id %v", err)
	}
}

func assertBlockFromReplies(t *testing.T, ctx context.Context, a *Client, peerID int64, msgID int32) {
	t.Helper()
	obj, err := a.Invoke(ctx, &mtproto.TLContactsBlockFromReplies{
		Constructor:   mtproto.CRC32_contacts_blockFromReplies,
		DeleteMessage: false,
		DeleteHistory: false,
		ReportSpam:    false,
		MsgId:         msgID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, up := range updatesOf(obj) {
		if up.GetPeerId().GetUserId() == peerID && up.GetBlocked_FLAGBOOLEAN() {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("blockFromReplies msg %d updates %#v", msgID, updatesOf(obj))
	}
	page, err := a.Invoke(ctx, &mtproto.TLContactsGetBlocked{
		Constructor: mtproto.CRC32_contacts_getBlocked_9a868f80,
		Offset:      0,
		Limit:       100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !blockedListHas(page, peerID) {
		t.Fatalf("getBlocked missing %d after msg %d: %T", peerID, msgID, page)
	}
}

func blockedListHas(obj mtproto.TLObject, userID int64) bool {
	var peers []*mtproto.PeerBlocked
	switch v := obj.(type) {
	case *mtproto.TLContactsBlocked:
		peers = v.GetBlocked()
	case *mtproto.TLContactsBlockedSlice:
		peers = v.GetBlocked()
	case *mtproto.Contacts_Blocked:
		peers = v.GetBlocked()
	}
	for _, peer := range peers {
		if peer.GetPeerId().GetUserId() == userID {
			return true
		}
	}
	return false
}

func deleteChannel(ctx context.Context, c *Client, channel *mtproto.InputChannel) {
	_, _ = c.Invoke(ctx, &mtproto.TLChannelsDeleteChannel{
		Constructor: mtproto.CRC32_channels_deleteChannel,
		Channel:     channel,
	})
}
