package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestPrivateChatReadOutbox(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	peer := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	assertReadOutbox(t, a, "user", bSelf.ID, peer, func(ctx context.Context, id int32) error {
		return b.ReadHistory(ctx, inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash), id)
	})
}

func TestBasicGroupReadOutbox(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	c, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	chatID, err := a.CreateChat(c, fmt.Sprintf("e2e-group-%d", time.Now().Unix()), inputUser(bSelf.ID, bSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChat(chatID)
	assertReadOutbox(t, a, "chat", chatID, peer, func(ctx context.Context, id int32) error {
		return b.ReadHistory(ctx, peer, id)
	})
}

func TestSupergroupReadOutbox(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	c, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	chID, chHash, err := a.CreateMegagroup(c, fmt.Sprintf("e2e-mega-%d", time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, chHash)
	if err := a.InviteToChannel(c, channel, inputUser(bSelf.ID, bSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChannel(chID, chHash)
	assertReadOutbox(t, a, "channel", chID, peer, func(ctx context.Context, id int32) error {
		var hash int64
		err := waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
			got, ok, err := b.ChatAccess(ctx, chID)
			if err != nil || !ok {
				return false, err
			}
			hash = got
			return true, nil
		})
		if err != nil {
			return err
		}
		return b.ReadChannelHistory(ctx, inputChannel(chID, hash), id)
	})
}

func assertReadOutbox(t *testing.T, sender *Client, kind string, peerID int64, peer *mtproto.InputPeer, mark func(context.Context, int32) error) {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	text := fmt.Sprintf("outbox-%s-%d", kind, time.Now().UnixNano())
	msgID, err := sender.SendText(c, peer, text, nil)
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	var before int32
	err = waitUntil(c, 400*time.Millisecond, func() (bool, error) {
		read, ok, err := sender.OutboxMaxID(c, kind, peerID)
		if err != nil || !ok {
			return false, err
		}
		seen = true
		before = read
		return true, nil
	})
	if err != nil {
		t.Fatalf("dialog for %s %d never appeared: %v", kind, peerID, err)
	}
	if !seen {
		t.Fatalf("dialog for %s %d missing", kind, peerID)
	}
	if before >= msgID {
		t.Fatalf("read_outbox_max_id=%d already covers message %d before the other side read it (false double tick)", before, msgID)
	}
	if err := mark(c, msgID); err != nil {
		t.Fatal(err)
	}
	err = waitUntil(c, 400*time.Millisecond, func() (bool, error) {
		read, ok, err := sender.OutboxMaxID(c, kind, peerID)
		if err != nil || !ok {
			return false, err
		}
		return read >= msgID, nil
	})
	if err != nil {
		read, _, _ := sender.OutboxMaxID(context.Background(), kind, peerID)
		t.Fatalf("read_outbox_max_id stayed at %d after the peer read message %d: %v", read, msgID, err)
	}
}
