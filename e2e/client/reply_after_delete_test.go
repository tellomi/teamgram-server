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
