package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestReplyAfterDeleteKeepsHeader is row 256. After both sides delete the
// original private message, a later reply still carries reply_to for the
// other person.
func TestReplyAfterDeleteKeepsHeader(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	alice, aliceSelf := dialPhone(t, ctx, cfg, "9996610715", "DelAlice")
	bob, bobSelf := dialPhone(t, ctx, cfg, "9996610716", "DelBob")
	peerForA := inputPeerUser(bobSelf.ID, bobSelf.AccessHash)
	peerForB := inputPeerUser(aliceSelf.ID, aliceSelf.AccessHash)
	aliceID, err := alice.SendText(ctx, peerForA, "gone-256", nil)
	if err != nil {
		t.Fatal(err)
	}
	var bobID int32
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		msgs, err := bob.History(ctx, peerForB, 20)
		if err != nil {
			return false, err
		}
		bobID = idOfText(msgs, "gone-256")
		return bobID != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = alice.Invoke(ctx, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{aliceID},
	}); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		leftA, err := alice.History(ctx, peerForA, 20)
		if err != nil {
			return false, err
		}
		leftB, err := bob.History(ctx, peerForB, 20)
		if err != nil {
			return false, err
		}
		return idOfText(leftA, "gone-256") == 0 && idOfText(leftB, "gone-256") == 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = bob.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        peerForB,
		Message:     "after-256",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: bobID,
		}).To_InputReplyTo(),
	}); err != nil {
		t.Fatal(err)
	}
	var seen int32
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		msgs, err := alice.History(ctx, peerForA, 20)
		if err != nil {
			return false, err
		}
		for _, m := range msgs {
			if m.GetMessage() != "after-256" {
				continue
			}
			// Layer 232 puts the id in reply_to_msg_id (flags.4). Decode fills
			// ReplyToMsgId_FLAGINT32, not the legacy ReplyToMsgId field.
			header := m.GetReplyTo()
			if header == nil {
				seen = 0
				return false, nil
			}
			seen = header.GetFixedReplyToMsgId()
			return seen == aliceID, nil
		}
		return false, nil
	}); err != nil {
		t.Fatalf("alice did not receive reply_to (last=%d want=%d)", seen, aliceID)
	}
}

// TestBasicGroupReplyAfterDeleteDropsHeader is the group side of row 256.
// Including deleted rows is private-chat only. A basic-group reply to a
// revoked message drops reply_to and does not mark the recipient mentioned.
// Phones 9996610840 / 9996610841, code 11111, layer 232.
func TestBasicGroupReplyAfterDeleteDropsHeader(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	alice, _ := dialPhone(t, ctx, cfg, "9996610840", "GrpDelA")
	bob, _ := dialPhone(t, ctx, cfg, "9996610841", "GrpDelB")
	bobID, bobHash, err := alice.ImportPhone(ctx, "9996610841", "GrpDelB")
	if err != nil {
		t.Fatal(err)
	}
	chatID, err := alice.CreateChat(ctx, "grp-del-256", inputUser(bobID, bobHash))
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChat(chatID)
	aliceMsg, err := alice.SendText(ctx, peer, "grp-gone-256", nil)
	if err != nil {
		t.Fatal(err)
	}
	var bobMsg int32
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		msgs, err := bob.History(ctx, peer, 20)
		if err != nil {
			return false, err
		}
		bobMsg = idOfText(msgs, "grp-gone-256")
		return bobMsg != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = alice.Invoke(ctx, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{aliceMsg},
	}); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		leftA, err := alice.History(ctx, peer, 20)
		if err != nil {
			return false, err
		}
		leftB, err := bob.History(ctx, peer, 20)
		if err != nil {
			return false, err
		}
		return idOfText(leftA, "grp-gone-256") == 0 && idOfText(leftB, "grp-gone-256") == 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = bob.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        peer,
		Message:     "grp-after-256",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: bobMsg,
		}).To_InputReplyTo(),
	}); err != nil {
		t.Fatal(err)
	}
	var reply *mtproto.Message
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		msgs, err := alice.History(ctx, peer, 20)
		if err != nil {
			return false, err
		}
		for _, m := range msgs {
			if m.GetMessage() == "grp-after-256" {
				reply = m
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal("alice did not receive the group reply")
	}
	header := reply.GetReplyTo()
	if header != nil && header.GetFixedReplyToMsgId() != 0 {
		t.Fatalf("basic group kept reply_to %d", header.GetFixedReplyToMsgId())
	}
	if reply.GetMentioned() {
		t.Fatal("basic group reply to a deleted message marked a mention")
	}
}
