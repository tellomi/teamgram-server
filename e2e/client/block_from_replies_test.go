package e2e

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestBlockFromRepliesRejectsDiscussionIDs is ISSUE-S-BLOCK-FROM-REPLIES-001
// (问题清单 369) on an isolated stack. contacts.blockFromReplies msg_id is
// the caller's message id in the private dialog with @replies
// (user 1271266957). A channel post, a messages.getDiscussionMessage id, a
// discussion-group message, a supergroup reply, and a private message are
// not that id. Those calls return 400 MSG_ID_INVALID and do not block or
// delete. Phones 9996610911 and 9996610912, code 11111. This test does not
// set TELLOMI_CHANNEL_COMMENTS.
func TestBlockFromRepliesRejectsDiscussionIDs(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	owner, ownerSelf := dialPhone(t, ctx, cfg, "9996610911", "BfrA")
	guest, guestSelf := dialPhone(t, ctx, cfg, "9996610912", "BfrB")
	guestID, guestHash, err := owner.ImportPhone(ctx, "9996610912", "BfrB")
	if err != nil {
		t.Fatal(err)
	}
	if guestID != guestSelf.ID || ownerSelf.ID == 0 || ownerSelf.ID == guestID {
		t.Fatalf("users owner %d guest %d imported %d", ownerSelf.ID, guestSelf.ID, guestID)
	}

	beforeOwner := repliesDialog(t, ctx, owner)
	beforeGuest := repliesDialog(t, ctx, guest)

	channelID, channelHash := createBroadcast(t, ctx, owner, "bfr-news")
	groupID, groupHash, err := owner.CreateMegagroup(ctx, "bfr-talk")
	if err != nil {
		t.Fatal(err)
	}
	roomID, roomHash, err := owner.CreateMegagroup(ctx, "bfr-room")
	if err != nil {
		t.Fatal(err)
	}
	cleanCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 20*time.Second)
	}
	t.Cleanup(func() {
		cctx, cancel := cleanCtx()
		defer cancel()
		deleteOwnedChannel(owner, cctx, channelID, channelHash)
		deleteOwnedChannel(owner, cctx, groupID, groupHash)
		deleteOwnedChannel(owner, cctx, roomID, roomHash)
	})

	setDiscussion(t, ctx, owner, inputChannel(channelID, channelHash), inputChannel(groupID, groupHash))
	postID, err := owner.SendText(ctx, inputPeerChannel(channelID, channelHash), "bfr-post", nil)
	if err != nil {
		t.Fatal(err)
	}
	discIDs := discussionIDs(t, ctx, owner, channelID, channelHash, postID)
	if len(discIDs) == 0 {
		t.Fatal("getDiscussionMessage returned no ids")
	}

	if err = owner.InviteToChannel(ctx, inputChannel(groupID, groupHash), inputUser(guestID, guestHash)); err != nil {
		t.Fatal(err)
	}
	if err = owner.InviteToChannel(ctx, inputChannel(roomID, roomHash), inputUser(guestID, guestHash)); err != nil {
		t.Fatal(err)
	}
	var guestGroup, guestRoom int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		var ok1, ok2 bool
		var err error
		guestGroup, ok1, err = guest.ChatAccess(ctx, groupID)
		if err != nil || !ok1 {
			return false, err
		}
		guestRoom, ok2, err = guest.ChatAccess(ctx, roomID)
		if err != nil || !ok2 {
			return false, err
		}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}

	commentID, err := guest.SendText(ctx, inputPeerChannel(groupID, guestGroup), "bfr-comment", nil)
	if err != nil {
		t.Fatal(err)
	}
	rootID, err := owner.SendText(ctx, inputPeerChannel(roomID, roomHash), "bfr-root", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := guest.History(ctx, inputPeerChannel(roomID, guestRoom), 20)
		if err != nil {
			return false, err
		}
		return hasMessageID(msgs, rootID), nil
	}); err != nil {
		t.Fatal(err)
	}
	replyObj, err := guest.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerChannel(roomID, guestRoom),
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
		t.Fatal("supergroup reply has no id")
	}
	privateID, err := owner.SendText(ctx, inputPeerUser(guestID, guestHash), "bfr-dm", nil)
	if err != nil {
		t.Fatal(err)
	}

	candidates := append([]int32{postID, commentID, rootID, replyID, privateID, 0, -1}, discIDs...)
	for _, id := range candidates {
		_, err = owner.Invoke(ctx, blockFromReplies(true, true, true, id))
		expectMsgIDInvalid(t, err)
		_, err = guest.Invoke(ctx, blockFromReplies(true, true, true, id))
		expectMsgIDInvalid(t, err)
	}

	assertNotBlocked(t, ctx, owner)
	assertNotBlocked(t, ctx, guest)
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		comment, err := owner.History(ctx, inputPeerChannel(groupID, groupHash), 20)
		if err != nil || !hasMessageID(comment, commentID) {
			return false, err
		}
		room, err := owner.History(ctx, inputPeerChannel(roomID, roomHash), 20)
		if err != nil || !hasMessageID(room, rootID) || !hasMessageID(room, replyID) {
			return false, err
		}
		dm, err := owner.History(ctx, inputPeerUser(guestID, guestHash), 20)
		if err != nil || !hasMessageID(dm, privateID) {
			return false, err
		}
		posts, err := owner.History(ctx, inputPeerChannel(channelID, channelHash), 20)
		if err != nil || !hasMessageID(posts, postID) {
			return false, err
		}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}

	afterOwner := repliesDialog(t, ctx, owner)
	afterGuest := repliesDialog(t, ctx, guest)
	if !sameDialog(beforeOwner, afterOwner) || !sameDialog(beforeGuest, afterGuest) {
		t.Fatalf("replies dialog changed owner %s->%s guest %s->%s", beforeOwner.summary(), afterOwner.summary(), beforeGuest.summary(), afterGuest.summary())
	}
}

const repliesServiceUser int64 = 1271266957

type dialogView struct {
	code    int
	message string
	ids     []int32
}

func (v dialogView) summary() string {
	if v.message != "" {
		return v.message
	}
	return "open"
}

func repliesDialog(t *testing.T, ctx context.Context, c *Client) dialogView {
	t.Helper()
	msgs, err := c.History(ctx, inputPeerUser(repliesServiceUser, 0), 20)
	if err != nil {
		rpc, ok := asRPCError(err)
		if !ok || rpc.Code >= 500 {
			t.Fatal(err)
		}
		return dialogView{code: rpc.Code, message: rpc.Message}
	}
	return dialogView{ids: messageIDs(msgs)}
}

func sameDialog(a, b dialogView) bool {
	if a.code != b.code || a.message != b.message || len(a.ids) != len(b.ids) {
		return false
	}
	seen := map[int32]int{}
	for _, id := range a.ids {
		seen[id]++
	}
	for _, id := range b.ids {
		seen[id]--
		if seen[id] < 0 {
			return false
		}
	}
	return true
}

func discussionIDs(t *testing.T, ctx context.Context, c *Client, channelID, accessHash int64, msgID int32) []int32 {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetDiscussionMessage{
		Constructor: mtproto.CRC32_messages_getDiscussionMessage,
		Peer:        inputPeerChannel(channelID, accessHash),
		MsgId:       msgID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var msgs []*mtproto.Message
	switch v := obj.(type) {
	case *mtproto.TLMessagesDiscussionMessage:
		if v.GetData2() != nil {
			msgs = v.GetData2().GetMessages()
		}
	case *mtproto.Messages_DiscussionMessage:
		msgs = v.GetMessages()
	default:
		t.Fatalf("getDiscussionMessage %T", obj)
	}
	return messageIDs(msgs)
}

// TestBlockFromRepliesDeletesCallerReplies seeds real rows in the caller's
// dialog with @replies and calls contacts.blockFromReplies. delete_message
// removes that box id. delete_history also removes the other live row from
// the same author. report_spam does not delete. A channel post, a private
// message, and another user's box id stay MSG_ID_INVALID.
// Phones 9996610913–9996610915, code 11111.
func TestBlockFromRepliesDeletesCallerReplies(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	owner, ownerSelf := dialPhone(t, ctx, cfg, "9996610913", "BfrC")
	author, authorSelf := dialPhone(t, ctx, cfg, "9996610914", "BfrD")
	_, otherSelf := dialPhone(t, ctx, cfg, "9996610915", "BfrE")
	if ownerSelf.ID == 0 || authorSelf.ID == 0 || otherSelf.ID == 0 || ownerSelf.ID == authorSelf.ID || authorSelf.ID == otherSelf.ID {
		t.Fatalf("users %d %d %d", ownerSelf.ID, authorSelf.ID, otherSelf.ID)
	}
	channelID, channelHash := createBroadcast(t, ctx, owner, "bfr-del-news")
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		deleteOwnedChannel(owner, cctx, channelID, channelHash)
		cleanupRepliesRows(ownerSelf.ID)
	})
	postID, err := owner.SendText(ctx, inputPeerChannel(channelID, channelHash), "bfr-del-post", nil)
	if err != nil {
		t.Fatal(err)
	}
	authorID, authorHash, err := owner.ImportPhone(ctx, "9996610914", "BfrD")
	if err != nil {
		t.Fatal(err)
	}
	if authorID != authorSelf.ID {
		t.Fatalf("author %d login %d", authorID, authorSelf.ID)
	}
	privateID, err := owner.SendText(ctx, inputPeerUser(authorID, authorHash), "bfr-del-dm", nil)
	if err != nil {
		t.Fatal(err)
	}
	const (
		one   int32 = 880040
		two   int32 = 880030
		extra int32 = 880050
		kept  int32 = 880020
	)
	seedRepliesRow(t, ownerSelf.ID, one, authorSelf.ID)
	seedRepliesRow(t, ownerSelf.ID, two, authorSelf.ID)
	seedRepliesRow(t, ownerSelf.ID, extra, authorSelf.ID)
	seedRepliesRow(t, ownerSelf.ID, kept, otherSelf.ID)

	for _, id := range []int32{postID, privateID, 0, -1, one} {
		if id == one {
			_, err = author.Invoke(ctx, blockFromReplies(true, true, true, id))
		} else {
			_, err = owner.Invoke(ctx, blockFromReplies(true, true, true, id))
		}
		expectMsgIDInvalid(t, err)
	}
	if deletedFlag(t, ownerSelf.ID, one) != 0 || deletedFlag(t, ownerSelf.ID, two) != 0 || deletedFlag(t, ownerSelf.ID, extra) != 0 || deletedFlag(t, ownerSelf.ID, kept) != 0 {
		t.Fatal("rejected ids changed a replies row")
	}
	posts, err := owner.History(ctx, inputPeerChannel(channelID, channelHash), 20)
	if err != nil || !hasMessageID(posts, postID) {
		t.Fatal(err)
	}
	dm, err := owner.History(ctx, inputPeerUser(authorID, authorHash), 20)
	if err != nil || !hasMessageID(dm, privateID) {
		t.Fatal(err)
	}

	if _, err = owner.Invoke(ctx, blockFromReplies(true, false, false, one)); err != nil {
		t.Fatal(err)
	}
	if deletedFlag(t, ownerSelf.ID, one) != 1 || deletedFlag(t, ownerSelf.ID, two) != 0 || deletedFlag(t, ownerSelf.ID, extra) != 0 || deletedFlag(t, ownerSelf.ID, kept) != 0 {
		t.Fatal("delete_message removed more than the one id")
	}
	_, err = owner.Invoke(ctx, blockFromReplies(false, false, false, one))
	expectMsgIDInvalid(t, err)
	if _, err = owner.Invoke(ctx, blockFromReplies(false, false, true, kept)); err != nil {
		t.Fatal(err)
	}
	if deletedFlag(t, ownerSelf.ID, kept) != 0 || deletedFlag(t, ownerSelf.ID, two) != 0 {
		t.Fatal("report_spam deleted a row")
	}
	blocked := blockedUserIDs(t, ctx, owner)
	if !containsID(blocked, authorSelf.ID) || !containsID(blocked, otherSelf.ID) {
		t.Fatalf("blocked %v", blocked)
	}
	if _, err = owner.Invoke(ctx, blockFromReplies(false, true, false, two)); err != nil {
		t.Fatal(err)
	}
	if deletedFlag(t, ownerSelf.ID, two) != 1 || deletedFlag(t, ownerSelf.ID, extra) != 1 || deletedFlag(t, ownerSelf.ID, kept) != 0 {
		t.Fatal("delete_history did not stay on the same author")
	}
	_, err = owner.Invoke(ctx, blockFromReplies(false, false, false, two))
	expectMsgIDInvalid(t, err)
	if deletedFlag(t, ownerSelf.ID, kept) != 0 {
		t.Fatal("other sender row was deleted")
	}
	dm, err = owner.History(ctx, inputPeerUser(authorID, authorHash), 20)
	if err != nil || !hasMessageID(dm, privateID) {
		t.Fatal(err)
	}
}

func seedRepliesRow(t *testing.T, owner int64, box int32, author int64) {
	t.Helper()
	did := mtproto.MakeDialogId(owner, mtproto.PEER_USER, repliesServiceUser)
	payload := fmt.Sprintf(`{"predicate_name":"message","fwd_from":{"from_id":{"predicate_name":"peerUser","user_id":%d}}}`, author)
	id := int64(900000000000) + int64(box)
	q := fmt.Sprintf(`INSERT INTO messages (id, user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_data, message, date2, deleted) VALUES (%d, %d, %d, %d, %d, %d, %d, %d, %d, %d, '%s', '', %d, 0)`,
		id, owner, box, did.A, did.B, box, repliesServiceUser, mtproto.PEER_USER, repliesServiceUser, id, payload, box)
	mustSQL(t, q)
}

func deletedFlag(t *testing.T, owner int64, box int32) int {
	t.Helper()
	out := strings.TrimSpace(mustSQL(t, fmt.Sprintf(`SELECT deleted FROM messages WHERE user_id=%d AND user_message_box_id=%d`, owner, box)))
	n, err := strconv.Atoi(out)
	if err != nil {
		t.Fatalf("deleted flag %q", out)
	}
	return n
}

func cleanupRepliesRows(owner int64) {
	_, _ = runSQL(fmt.Sprintf(`DELETE FROM messages WHERE user_id=%d AND user_message_box_id IN (880040, 880030, 880050, 880020)`, owner))
}

func blockedUserIDs(t *testing.T, ctx context.Context, c *Client) []int64 {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLContactsGetBlocked{
		Constructor: mtproto.CRC32_contacts_getBlocked_9a868f80,
		Offset:      0,
		Limit:       100,
	})
	if err != nil {
		t.Fatal(err)
	}
	var rows []*mtproto.PeerBlocked
	switch v := obj.(type) {
	case *mtproto.TLContactsBlocked:
		if v.GetData2() != nil {
			rows = v.GetData2().GetBlocked()
		}
	case *mtproto.TLContactsBlockedSlice:
		if v.GetData2() != nil {
			rows = v.GetData2().GetBlocked()
		}
	case *mtproto.Contacts_Blocked:
		rows = v.GetBlocked()
	default:
		t.Fatalf("getBlocked %T", obj)
	}
	var ids []int64
	for _, row := range rows {
		if row.GetPeerId().GetUserId() != 0 {
			ids = append(ids, row.GetPeerId().GetUserId())
		}
	}
	return ids
}

func containsID(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func blockFromReplies(deleteMessage, deleteHistory, reportSpam bool, id int32) *mtproto.TLContactsBlockFromReplies {
	return &mtproto.TLContactsBlockFromReplies{
		Constructor:   mtproto.CRC32_contacts_blockFromReplies,
		DeleteMessage: deleteMessage,
		DeleteHistory: deleteHistory,
		ReportSpam:    reportSpam,
		MsgId:         id,
	}
}

func expectMsgIDInvalid(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("blockFromReplies succeeded")
	}
	rpc, ok := asRPCError(err)
	if !ok || rpc.Code != 400 || rpc.Message != "MSG_ID_INVALID" {
		if ok {
			t.Fatalf("blockFromReplies %d %s", rpc.Code, rpc.Message)
		}
		t.Fatal(err)
	}
}

func assertNotBlocked(t *testing.T, ctx context.Context, c *Client) {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLContactsGetBlocked{
		Constructor: mtproto.CRC32_contacts_getBlocked_9a868f80,
		Offset:      0,
		Limit:       100,
	})
	if err != nil {
		t.Fatal(err)
	}
	var box *mtproto.Contacts_Blocked
	switch v := obj.(type) {
	case *mtproto.TLContactsBlocked:
		box = v.GetData2()
	case *mtproto.TLContactsBlockedSlice:
		box = v.GetData2()
	case *mtproto.Contacts_Blocked:
		box = v
	default:
		t.Fatalf("getBlocked %T", obj)
	}
	if box == nil || len(box.GetBlocked()) != 0 || box.GetCount() != 0 {
		n := 0
		if box != nil {
			n = len(box.GetBlocked())
		}
		t.Fatalf("blocked rows %d", n)
	}
}

func deleteOwnedChannel(c *Client, ctx context.Context, id, hash int64) {
	_, _ = c.Invoke(ctx, &mtproto.TLChannelsDeleteChannel{
		Constructor: mtproto.CRC32_channels_deleteChannel,
		Channel:     inputChannel(id, hash),
	})
}

func messageIDs(msgs []*mtproto.Message) []int32 {
	var ids []int32
	for _, m := range msgs {
		if m != nil && m.GetId() != 0 {
			ids = append(ids, m.GetId())
		}
	}
	return ids
}

func hasMessageID(msgs []*mtproto.Message, id int32) bool {
	for _, got := range messageIDs(msgs) {
		if got == id {
			return true
		}
	}
	return false
}
