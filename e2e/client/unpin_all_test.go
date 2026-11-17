package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestUnpinAllClearsEveryCopy covers ISSUE-S-UNPINALL-MULTI-001 (row 234),
// ISSUE-S-UNPINALL-PRIV-001 (row 235), and ISSUE-S-TOPIC-UNPIN-001 (row 259).
// Unpin-all used to clear only the last channel pin id. The private peer
// was told the messages were unpinned while their rows stayed pinned.
func TestUnpinAllClearsEveryCopy(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()

	a, _ := dialPhone(t, ctx, cfg, "9996610445", "PinA")
	b, _ := dialPhone(t, ctx, cfg, "9996610446", "PinB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610446", "PinB")
	if err != nil {
		t.Fatal(err)
	}
	aID, aHash, err := b.ImportPhone(ctx, "9996610445", "PinA")
	if err != nil {
		t.Fatal(err)
	}
	toB := inputPeerUser(bID, bHash)
	toA := inputPeerUser(aID, aHash)
	first, err := a.SendText(ctx, toB, "pin-one", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.SendText(ctx, toB, "pin-two", nil)
	if err != nil {
		t.Fatal(err)
	}
	pinMessage(t, a, toB, first, false)
	pinMessage(t, a, toB, second, false)
	unpinAll(t, a, toB)

	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return !historyPinned(t, a, toB, first) && !historyPinned(t, a, toB, second) &&
			!historyPinned(t, b, toA, 0), nil
	}); err != nil {
		t.Fatalf("private unpin left a pin: %v", err)
	}
	if len(pinnedSearch(t, a, toB)) != 0 || len(pinnedSearch(t, b, toA)) != 0 {
		t.Fatal("private pinned search still hits")
	}

	chID, chHash, err := a.CreateMegagroup(ctx, "unpin-mega")
	if err != nil {
		t.Fatal(err)
	}
	mega := inputPeerChannel(chID, chHash)
	m1, err := a.SendText(ctx, mega, "mega-one", nil)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := a.SendText(ctx, mega, "mega-two", nil)
	if err != nil {
		t.Fatal(err)
	}
	pinMessage(t, a, mega, m1, false)
	pinMessage(t, a, mega, m2, false)
	pinMessage(t, a, mega, m1, true)
	if historyPinned(t, a, mega, m1) || len(pinnedSearch(t, a, mega)) != 1 {
		t.Fatal("unpinning one megagroup message left it pinned or dropped the other")
	}
	unpinAll(t, a, mega)
	if historyPinned(t, a, mega, m2) || len(pinnedSearch(t, a, mega)) != 0 {
		t.Fatal("megagroup unpinAll left a pin")
	}
}

func pinnedSearch(t *testing.T, c *Client, peer *mtproto.InputPeer) []*mtproto.Message {
	t.Helper()
	res := invokeOK(t, c, &mtproto.TLMessagesSearch{
		Constructor: mtproto.CRC32_messages_search_a0fda762,
		Peer:        peer,
		Filter:      pinnedFilter(),
		Limit:       20,
	})
	msgs, ok := messagesOf(res)
	if !ok {
		t.Fatalf("messages.search returned %T", res)
	}
	return msgs
}

func pinnedFilter() *mtproto.MessagesFilter {
	return mtproto.MakeTLInputMessagesFilterPinned(&mtproto.MessagesFilter{}).To_MessagesFilter()
}

func pinMessage(t *testing.T, c *Client, peer *mtproto.InputPeer, id int32, unpin bool) {
	t.Helper()
	invokeOK(t, c, &mtproto.TLMessagesUpdatePinnedMessage{
		Constructor: mtproto.CRC32_messages_updatePinnedMessage,
		Peer:        peer,
		Id:          id,
		Unpin:       unpin,
	})
}

func unpinAll(t *testing.T, c *Client, peer *mtproto.InputPeer) {
	t.Helper()
	invokeOK(t, c, &mtproto.TLMessagesUnpinAllMessages{
		Constructor: mtproto.CRC32_messages_unpinAllMessages_f025bc8b,
		Peer:        peer,
	})
}

func historyPinned(t *testing.T, c *Client, peer *mtproto.InputPeer, id int32) bool {
	t.Helper()
	ctx, cancel := contextTimeout(15 * time.Second)
	defer cancel()
	msgs, err := c.History(ctx, peer, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if id != 0 && m.GetId() != id {
			continue
		}
		if m.GetPinned() {
			return true
		}
	}
	return false
}
