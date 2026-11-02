package chatbox

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// fakeIDs is one private chat where user 4 numbers the messages 250, 251 and
// user 9 numbers the same messages 2734, 2735.
type fakeIDs struct{ box map[int64]map[int32]int64 }

func (f fakeIDs) DialogIDs(_ context.Context, user int64, ids []int32) (map[int32]int64, error) {
	out := map[int32]int64{}
	for _, id := range ids {
		if d, ok := f.box[user][id]; ok {
			out[id] = d
		}
	}
	return out, nil
}

func (f fakeIDs) BoxIDs(_ context.Context, user int64, dlgs []int64) (map[int64]int32, error) {
	out := map[int64]int32{}
	for _, d := range dlgs {
		for id, v := range f.box[user] {
			if v == d {
				out[d] = id
			}
		}
	}
	return out, nil
}

func TestPrivateReactionUsesEachSidesMessageID(t *testing.T) {
	b := newBook(nil)
	b.ids = fakeIDs{box: map[int64]map[int32]int64{
		4: {250: 7001, 251: 7002},
		9: {2734: 7001, 2735: 7002},
	}}
	ctx := context.Background()
	peer4 := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	peer9 := mtproto.MakePeerUtil(mtproto.PEER_USER, 4)

	up, err := b.React(ctx, 9, peer9, 2734, []string{"👍"}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := up.Updates[0].GetMsgId_INT32(); got != 2734 {
		t.Fatalf("reactor update on %d", got)
	}
	if n := len(up.Updates[0].GetReactions_MESSAGEREACTIONS().GetResults()); n != 1 {
		t.Fatalf("reactor update results %d", n)
	}

	// The author sees the reaction on its own id for the message, not on 2734.
	msgs := []*mtproto.Message{
		mtproto.MakeTLMessage(&mtproto.Message{Id: 250}).To_Message(),
		mtproto.MakeTLMessage(&mtproto.Message{Id: 251}).To_Message(),
	}
	b.ApplyReactions(ctx, 4, peer4, msgs)
	if !hasEmoji(msgs[0], "👍") || msgs[1].GetReactions() != nil {
		t.Fatalf("author view %v / %v", msgs[0].GetReactions(), msgs[1].GetReactions())
	}
	mine := []*mtproto.Message{mtproto.MakeTLMessage(&mtproto.Message{Id: 2734}).To_Message()}
	b.ApplyReactions(ctx, 9, peer9, mine)
	if !hasEmoji(mine[0], "👍") {
		t.Fatalf("reactor view %v", mine[0].GetReactions())
	}

	// Both sides add 👍 to the same message: one result with count 2.
	if _, err := b.React(ctx, 4, peer4, 250, []string{"👍"}, nil, false); err != nil {
		t.Fatal(err)
	}
	for _, side := range []struct {
		self int64
		peer *mtproto.PeerUtil
		id   int32
	}{{4, peer4, 250}, {9, peer9, 2734}} {
		up, err := b.ReactionsFor(ctx, side.self, side.peer, []int32{side.id})
		if err != nil {
			t.Fatal(err)
		}
		res := up.Updates[0].GetReactions_MESSAGEREACTIONS().GetResults()
		if len(res) != 1 || res[0].GetCount() != 2 {
			t.Fatalf("user %d sees %v", side.self, res)
		}
	}
	list, err := b.ReactionList(ctx, 4, peer4, 250, nil, "", 10)
	if err != nil || list.Count != 2 {
		t.Fatalf("list %v %v", list, err)
	}
	unread, err := b.UnreadReactions(ctx, 4, peer4, 0, 0, 10, 0, 0)
	if err != nil || len(unread) != 1 || unread[0].GetId() != 250 {
		t.Fatalf("unread %v %v", unread, err)
	}

	// Replacing a reaction from the other side still finds the old one.
	if _, err := b.React(ctx, 9, peer9, 2734, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	up, err = b.ReactionsFor(ctx, 4, peer4, []int32{250})
	if err != nil {
		t.Fatal(err)
	}
	if res := up.Updates[0].GetReactions_MESSAGEREACTIONS().GetResults(); len(res) != 1 || res[0].GetCount() != 1 {
		t.Fatalf("after 9 removed: %v", res)
	}
}

func hasEmoji(m *mtproto.Message, emo string) bool {
	for _, rc := range m.GetReactions().GetResults() {
		if rc.GetReaction_STRING() == emo && rc.GetCount() > 0 {
			return true
		}
	}
	return false
}
