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
	// Own users only. Runs alongside session revocation on the shared stack.
	t.Parallel()
	s := useReliabilityStack(t)
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

	beforePts, beforeQts, beforeDate := matchedState(t, a1, a2)

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

	// Private chats number a message separately on each side. The peer reads
	// its own id. The sender's read_outbox_max_id then covers the sender's id.
	var peerMsg int32
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := peerClient.History(ctx, back, 20)
		if err != nil {
			return false, err
		}
		peerMsg = idOfText(msgs, "sync-edited")
		return peerMsg != 0, nil
	}); err != nil {
		t.Fatalf("peer never received the edited message: %v", err)
	}
	if err = peerClient.ReadHistory(ctx, back, peerMsg); err != nil {
		t.Fatal(err)
	}
	assertSamePts(t, a1, a2, beforePts)
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		d1 := findDialog(t, ctx, a1, peerID, 0)
		d2 := findDialog(t, ctx, a2, peerID, 0)
		if d1 == nil || d2 == nil {
			return false, nil
		}
		read1 := d1.GetReadOutboxMaxId()
		read2 := d2.GetReadOutboxMaxId()
		return read1 == read2 && read1 >= msgID, nil
	}); err != nil {
		t.Fatalf("read_outbox_max_id %d and %d, want both >= %d: %v", outboxMax(findDialog(t, ctx, a1, peerID, 0)), outboxMax(findDialog(t, ctx, a2, peerID, 0)), msgID, err)
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

func matchedState(t *testing.T, a, b *Client) (pts, qts, date int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var lastA, lastB int32
	err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		var err error
		pts, qts, date, err = a.State(ctx)
		if err != nil {
			return false, err
		}
		var q2, d2 int32
		lastB, q2, d2, err = b.State(ctx)
		if err != nil {
			return false, err
		}
		lastA = pts
		_ = q2
		_ = d2
		return pts == lastB, nil
	})
	if err != nil {
		t.Fatalf("pts %d and %d did not match: %v", lastA, lastB, err)
	}
	return pts, qts, date
}

func outboxMax(d *mtproto.Dialog) int32 {
	if d == nil {
		return -1
	}
	return d.GetReadOutboxMaxId()
}

func assertSamePts(t *testing.T, a, b *Client, before int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var lastA, lastB int32
	err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		pa, _, _, err := a.State(ctx)
		if err != nil {
			return false, err
		}
		pb, _, _, err := b.State(ctx)
		if err != nil {
			return false, err
		}
		lastA, lastB = pa, pb
		return pa == pb && pa > before, nil
	})
	if err != nil {
		t.Fatalf("pts %d and %d, want the same value above %d: %v", lastA, lastB, before, err)
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
