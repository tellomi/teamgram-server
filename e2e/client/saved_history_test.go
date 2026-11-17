package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestDeleteSavedHistoryAndTagSearch covers ISSUE-S-SAVED-DELETE-001 (行 140)
// and ISSUE-S-SAVED-TAGSEARCH-001 (行 141). Phones 9996610647 / 9996610648,
// code 11111, layer 232.
func TestDeleteSavedHistoryAndTagSearch(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610647", "SavedA")
	b, _ := dialPhone(t, ctx, cfg, "9996610648", "SavedB")
	if _, _, _, err := b.State(ctx); err != nil {
		t.Fatal(err)
	}
	bID, bHash, err := a.ImportPhone(ctx, "9996610648", "SavedB")
	if err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("saved-src-%d", time.Now().UnixNano())
	msgID, err := a.SendText(ctx, inputPeerUser(bID, bHash), text, nil)
	if err != nil {
		t.Fatal(err)
	}
	fwd, err := a.Invoke(ctx, &mtproto.TLMessagesForwardMessages{
		Constructor: mtproto.CRC32_messages_forwardMessages_13704a7c,
		FromPeer:    inputPeerUser(bID, bHash),
		Id:          []int32{msgID},
		RandomId:    []int64{randomID()},
		ToPeer:      inputPeerSelf(),
	})
	if err != nil {
		t.Fatal(err)
	}
	savedID := messageID(fwd)
	if savedID == 0 {
		t.Fatalf("forward %T", fwd)
	}
	if _, err = a.SendReaction(ctx, inputPeerSelf(), savedID, "👍"); err != nil {
		t.Fatal(err)
	}
	found, err := a.Invoke(ctx, &mtproto.TLMessagesSearch{
		Constructor: mtproto.CRC32_messages_search_29ee847a,
		Peer:        inputPeerSelf(),
		Q:           "",
		SavedReaction: []*mtproto.Reaction{
			mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
		},
		Filter: mtproto.MakeTLInputMessagesFilterEmpty(nil).To_MessagesFilter(),
		Limit:  20,
	})
	if err != nil {
		t.Fatalf("tag search: %v", err)
	}
	if !hasText(allMessages(found), text) {
		t.Fatalf("tag search missed %q in %T", text, found)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLMessagesDeleteSavedHistory{
		Constructor: mtproto.CRC32_messages_deleteSavedHistory_4dc5085f,
		Peer:        inputPeerUser(bID, bHash),
		MaxId:       0,
	}); err != nil {
		t.Fatal(err)
	}
	hist, err := a.Invoke(ctx, &mtproto.TLMessagesGetSavedHistory{
		Constructor: mtproto.CRC32_messages_getSavedHistory_998ab009,
		Peer:        inputPeerUser(bID, bHash),
		Limit:       20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasText(allMessages(hist), text) {
		t.Fatal("deleteSavedHistory left the message")
	}
}
