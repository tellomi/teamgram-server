package chatbox

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPinnedMessageKeepsReactionEmoji(t *testing.T) {
	b := newBook(nil)
	ctx := context.Background()
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: 20}
	ups, err := b.React(ctx, 4, peer, 15, []string{"🔥"}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	var count int32
	for _, up := range ups.GetUpdates() {
		sum := up.GetReactions_MESSAGEREACTIONS()
		if sum == nil || len(sum.GetResults()) == 0 {
			continue
		}
		got = sum.GetResults()[0].GetReaction_REACTION().GetEmoticon()
		count = sum.GetResults()[0].GetCount()
	}
	if got != "🔥" || count != 1 {
		t.Fatalf("update emoji %q count %d", got, count)
	}
	pinned := mtproto.MakeTLMessage(&mtproto.Message{Id: 15, Pinned: true}).To_Message()
	b.ApplyReactions(ctx, 4, peer, []*mtproto.Message{pinned})
	results := pinned.GetReactions().GetResults()
	if len(results) != 1 || results[0].GetReaction_REACTION().GetEmoticon() != "🔥" || results[0].GetCount() != 1 {
		t.Fatalf("pinned %#v", results)
	}
	if _, err = b.React(ctx, 8, peer, 15, []string{"🔥"}, nil, false); err != nil {
		t.Fatal(err)
	}
	pinned = mtproto.MakeTLMessage(&mtproto.Message{Id: 15, Pinned: true}).To_Message()
	b.ApplyReactions(ctx, 4, peer, []*mtproto.Message{pinned})
	results = pinned.GetReactions().GetResults()
	if len(results) != 1 || results[0].GetCount() != 2 || results[0].GetReaction_REACTION().GetEmoticon() != "🔥" {
		t.Fatalf("count %#v", results)
	}
}
