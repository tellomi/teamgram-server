package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestPrivateNoForwardsFlagsAndMessage is 行 137 / ISSUE-S-NOFWD-001.
// toggleNoForwards on a private chat sets both userFull flags. A message
// sent with noforwards cannot be forwarded.
// Phones 9996610653 / 9996610654, code 11111.
func TestPrivateNoForwardsFlagsAndMessage(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	a, aSelf := dialPhone(t, ctx, cfg, "9996610653", "NoFwdA")
	b, bSelf := dialPhone(t, ctx, cfg, "9996610654", "NoFwdB")
	if _, _, err := a.ImportPhone(ctx, "9996610654", "NoFwdB"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.ImportPhone(ctx, "9996610653", "NoFwdA"); err != nil {
		t.Fatal(err)
	}
	peerB := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	peerA := inputPeerUser(aSelf.ID, aSelf.AccessHash)

	if _, err := a.Invoke(ctx, &mtproto.TLMessagesToggleNoForwards{
		Constructor: mtproto.CRC32_messages_toggleNoForwards_b11eafa2,
		Peer:        peerB,
		Enabled:     mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	assertNoForwardFlags(t, ctx, a, inputUser(bSelf.ID, bSelf.AccessHash), true)
	assertNoForwardFlags(t, ctx, b, inputUser(aSelf.ID, aSelf.AccessHash), true)

	if _, err := a.Invoke(ctx, &mtproto.TLMessagesToggleNoForwards{
		Constructor: mtproto.CRC32_messages_toggleNoForwards_b11eafa2,
		Peer:        peerB,
		Enabled:     mtproto.BoolFalse,
	}); err != nil {
		t.Fatal(err)
	}
	assertNoForwardFlags(t, ctx, a, inputUser(bSelf.ID, bSelf.AccessHash), false)
	assertNoForwardFlags(t, ctx, b, inputUser(aSelf.ID, aSelf.AccessHash), false)

	text := fmt.Sprintf("nofwd-msg-%d", time.Now().UnixNano())
	sent, err := a.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        peerB,
		Message:     text,
		RandomId:    randomID(),
		Noforwards:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if messageID(sent) == 0 {
		t.Fatalf("send returned %T", sent)
	}
	var onB int32
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := b.History(ctx, peerA, 20)
		if err != nil {
			return false, err
		}
		for _, m := range msgs {
			if m.GetMessage() == text {
				if !m.GetNoforwards() {
					return false, fmt.Errorf("delivered message dropped noforwards")
				}
				onB = m.GetId()
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	expectForwardRefused(t, ctx, b, peerA, inputPeerSelf(), onB)
}

func assertNoForwardFlags(t *testing.T, ctx context.Context, c *Client, id *mtproto.InputUser, on bool) {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          id,
	})
	if err != nil {
		t.Fatal(err)
	}
	full := fullUserOf(t, obj)
	if full.GetNoforwardsMyEnabled() != on || full.GetNoforwardsPeerEnabled() != on {
		t.Fatalf("noforwards my=%v peer=%v, want %v", full.GetNoforwardsMyEnabled(), full.GetNoforwardsPeerEnabled(), on)
	}
}
