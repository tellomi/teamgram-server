//go:build reliability

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestReliabilityMultiDeviceSync is two sessions of one user. After send,
// edit, delete, and the peer's read, both sessions report the same pts and
// the same message or read state. The session that did not send the message
// can read it from updates.getDifference.
func TestReliabilityMultiDeviceSync(t *testing.T) {
	s := newReliabilityStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	phone := reliabilityPhone("0321")
	peerPhone := reliabilityPhone("0322")
	a1, _ := s.login(t, ProfileIOS, phone, "Sync")
	a2, _ := s.login(t, ProfileAndroid, phone, "Sync")
	_, peerSelf := s.login(t, ProfileIOS, peerPhone, "SyncPeer")
	peerID, peerHash, err := a1.ImportPhone(ctx, peerPhone.Number, "SyncPeer")
	if err != nil {
		t.Fatal(err)
	}
	if peerID != peerSelf.ID {
		t.Fatalf("imported peer %d, login %d", peerID, peerSelf.ID)
	}
	peer := inputPeerUser(peerID, peerHash)
	a1PeerHash := peerHash
	_, a2Hash, err := a2.ImportPhone(ctx, peerPhone.Number, "SyncPeer")
	if err != nil {
		t.Fatal(err)
	}
	if a2Hash == 0 {
		a2Hash = a1PeerHash
	}
	a2Peer := inputPeerUser(peerID, a2Hash)
	peerClient, _ := s.login(t, ProfileAndroid, peerPhone, "SyncPeer")
	selfID, selfHash, err := peerClient.ImportPhone(ctx, phone.Number, "Sync")
	if err != nil {
		t.Fatal(err)
	}
	back := inputPeerUser(selfID, selfHash)

	beforePts, beforeQts, beforeDate := callState(t, a1)
	pts2, _, _ := callState(t, a2)
	if beforePts != pts2 {
		t.Fatalf("pts before send %d and %d", beforePts, pts2)
	}

	msgID, err := a1.SendText(ctx, peer, "sync-body", nil)
	if err != nil {
		t.Fatal(err)
	}
	assertSamePts(t, a1, a2, beforePts)
	if got := historyText(t, ctx, a2, a2Peer, msgID); got != "sync-body" {
		t.Fatalf("other session history %q", got)
	}
	diff, err := a2.Difference(ctx, beforePts, beforeQts, beforeDate)
	if err != nil {
		t.Fatal(err)
	}
	if !differenceHasText(diff, "sync-body") {
		t.Fatalf("getDifference missed the send: %T", diff)
	}

	invokeOK(t, a1, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_b106e66c,
		Peer:        peer,
		Id:          msgID,
		Message:     &wrapperspb.StringValue{Value: "sync-edited"},
	})
	assertSamePts(t, a1, a2, beforePts)
	if got := historyText(t, ctx, a1, peer, msgID); got != "sync-edited" {
		t.Fatalf("sender history %q", got)
	}
	if got := historyText(t, ctx, a2, a2Peer, msgID); got != "sync-edited" {
		t.Fatalf("other session history after edit %q", got)
	}

	if err := peerClient.ReadHistory(ctx, back, msgID); err != nil {
		t.Fatal(err)
	}
	assertSamePts(t, a1, a2, beforePts)
	read1 := dialogFor(t, ctx, a1, peerID, 0).GetReadOutboxMaxId()
	read2 := dialogFor(t, ctx, a2, peerID, 0).GetReadOutboxMaxId()
	if read1 != msgID || read2 != msgID {
		t.Fatalf("read_outbox_max_id %d and %d, want %d", read1, read2, msgID)
	}

	invokeOK(t, a1, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{msgID},
	})
	assertSamePts(t, a1, a2, beforePts)
	if got := historyText(t, ctx, a1, peer, msgID); got != "" {
		t.Fatalf("sender still has %q", got)
	}
	if got := historyText(t, ctx, a2, a2Peer, msgID); got != "" {
		t.Fatalf("other session still has %q", got)
	}
}

func assertSamePts(t *testing.T, a, b *Client, before int32) {
	t.Helper()
	pa, _, _ := callState(t, a)
	pb, _, _ := callState(t, b)
	if pa != pb {
		t.Fatalf("pts %d and %d", pa, pb)
	}
	if pa <= before {
		t.Fatalf("pts %d did not advance past %d", pa, before)
	}
}

func historyText(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, id int32) string {
	t.Helper()
	msgs, err := c.History(ctx, peer, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if m.GetId() != id {
			continue
		}
		if m.GetPredicateName() == mtproto.Predicate_messageEmpty {
			return ""
		}
		return m.GetMessage()
	}
	return ""
}

func differenceHasText(obj mtproto.TLObject, text string) bool {
	var msgs []*mtproto.Message
	switch d := obj.(type) {
	case *mtproto.TLUpdatesDifference:
		msgs = d.GetNewMessages()
	case *mtproto.TLUpdatesDifferenceSlice:
		msgs = d.GetNewMessages()
	default:
		return false
	}
	for _, m := range msgs {
		if m.GetMessage() == text {
			return true
		}
	}
	return false
}
