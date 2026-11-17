package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestSearchTokensAndPrivateFromID covers ISSUE-S-SEARCH-MULTIWORD-001 (行 133)
// and ISSUE-S-SEARCH-FROMID-PRIV-001 (行 83). Phones 9996610649 / 9996610650,
// code 11111, layer 232.
func TestSearchTokensAndPrivateFromID(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610649", "FindA")
	b, _ := dialPhone(t, ctx, cfg, "9996610650", "FindB")
	if _, _, _, err := b.State(ctx); err != nil {
		t.Fatal(err)
	}
	bID, bHash, err := a.ImportPhone(ctx, "9996610650", "FindB")
	if err != nil {
		t.Fatal(err)
	}
	token := fmt.Sprintf("tok%d", time.Now().UnixNano()%1_000_000_000)
	text := token + " apple banana"
	peer := inputPeerUser(bID, bHash)
	if _, err = a.SendText(ctx, peer, text, nil); err != nil {
		t.Fatal(err)
	}
	found, err := a.Invoke(ctx, &mtproto.TLMessagesSearch{
		Constructor: mtproto.CRC32_messages_search_29ee847a,
		Peer:        peer,
		Q:           "banana " + token,
		Filter:      mtproto.MakeTLInputMessagesFilterEmpty(nil).To_MessagesFilter(),
		Limit:       20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasText(allMessages(found), text) {
		t.Fatalf("reordered words missed %q", text)
	}
	bySelf, err := a.Invoke(ctx, &mtproto.TLMessagesSearch{
		Constructor: mtproto.CRC32_messages_search_29ee847a,
		Peer:        peer,
		Q:           token,
		FromId:      inputPeerUser(aSelf.ID, aSelf.AccessHash),
		Filter:      mtproto.MakeTLInputMessagesFilterEmpty(nil).To_MessagesFilter(),
		Limit:       20,
	})
	if err != nil {
		t.Fatalf("from_id: %v", err)
	}
	if !hasText(allMessages(bySelf), text) {
		t.Fatal("from_id self missed the message")
	}
}
