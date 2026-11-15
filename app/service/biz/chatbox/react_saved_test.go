package chatbox

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestHasReactionFiltersSavedTag(t *testing.T) {
	b := newBook(nil)
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_SELF, 5)
	thumb := mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction()
	heart := mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "❤"}).To_Reaction()
	if _, err := b.React(ctx, 5, peer, 9, []string{"👍"}, nil, false); err != nil {
		t.Fatal(err)
	}
	if !b.HasReaction(ctx, 5, peer, 9, []*mtproto.Reaction{thumb}) {
		t.Fatal("tagged message dropped")
	}
	if b.HasReaction(ctx, 5, peer, 9, []*mtproto.Reaction{heart}) {
		t.Fatal("other emoji matched")
	}
	if b.HasReaction(ctx, 5, peer, 4, []*mtproto.Reaction{thumb}) {
		t.Fatal("untagged message matched")
	}
	if !b.HasReaction(ctx, 5, peer, 4, nil) {
		t.Fatal("empty filter must keep the message")
	}
}
