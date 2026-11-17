package chatbox

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestKeepSavedReactionFiltersEmptyQuery(t *testing.T) {
	ResetShared()
	b := Shared()
	ctx := context.Background()
	self := mtproto.MakePeerUtil(mtproto.PEER_SELF, 4)
	if _, err := b.React(ctx, 4, self, 10, []string{"👍"}, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := b.React(ctx, 4, self, 11, []string{"❤"}, nil, false); err != nil {
		t.Fatal(err)
	}
	boxes := []*mtproto.MessageBox{
		{MessageId: 10, Message: &mtproto.Message{Id: 10, Message: "tagged"}},
		{MessageId: 11, Message: &mtproto.Message{Id: 11, Message: "other"}},
		{MessageId: 12, Message: &mtproto.Message{Id: 12, Message: "plain"}},
	}
	want := []*mtproto.Reaction{mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction()}
	got := b.KeepSavedReaction(ctx, 4, boxes, want)
	if len(got) != 1 || got[0].GetMessage().GetId() != 10 {
		t.Fatalf("kept %#v", got)
	}
}
