package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestGetMessagesReplyToReturnsOriginal is row 205.
// inputMessageReplyTo returns the message that was replied to.
func TestGetMessagesReplyToReturnsOriginal(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	alice, aliceSelf := dialPhone(t, ctx, cfg, "9996610707", "ReplyAlice")
	bob, bobSelf := dialPhone(t, ctx, cfg, "9996610708", "ReplyBob")
	peerForA := inputPeerUser(bobSelf.ID, bobSelf.AccessHash)
	if _, err := alice.SendText(ctx, peerForA, "origin-row-205", nil); err != nil {
		t.Fatal(err)
	}
	var originOnB int32
	peerForB := inputPeerUser(aliceSelf.ID, aliceSelf.AccessHash)
	if err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		msgs, err := bob.History(ctx, peerForB, 20)
		if err != nil {
			return false, err
		}
		originOnB = idOfText(msgs, "origin-row-205")
		return originOnB != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	sent, err := bob.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        peerForB,
		Message:     "reply-row-205",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: originOnB,
		}).To_InputReplyTo(),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyID := messageID(sent)
	if replyID == 0 {
		t.Fatal("reply has no id")
	}
	obj, err := bob.Invoke(ctx, &mtproto.TLMessagesGetMessages{
		Constructor: mtproto.CRC32_messages_getMessages_63c66506,
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{
			mtproto.MakeTLInputMessageReplyTo(&mtproto.InputMessage{Id: replyID}).To_InputMessage(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs, ok := messagesOf(obj)
	if !ok || len(msgs) != 1 || msgs[0].GetId() != originOnB || msgs[0].GetMessage() != "origin-row-205" {
		t.Fatalf("getMessages reply-to: ok=%v %#v", ok, msgs)
	}
}
