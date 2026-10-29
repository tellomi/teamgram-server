package catalog

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestTopAndRecentReactions(t *testing.T) {
	s := newService(builtinPacks())
	s.now = func() int32 { return 100 }

	top := s.TopReactions(0, 0)
	if top.GetPredicateName() != mtproto.Predicate_messages_reactions {
		t.Fatalf("top predicate %s", top.GetPredicateName())
	}
	def := defaultReactionOrder()
	if len(top.GetReactions()) != len(def) {
		t.Fatalf("top len %d want %d", len(top.GetReactions()), len(def))
	}
	for i, emo := range def {
		if top.GetReactions()[i].GetEmoticon() != emo {
			t.Fatalf("default order[%d] = %s, want %s", i, top.GetReactions()[i].GetEmoticon(), emo)
		}
	}
	same := s.TopReactions(0, top.GetHash())
	if same.GetPredicateName() != mtproto.Predicate_messages_reactionsNotModified {
		t.Fatalf("top hash predicate %s", same.GetPredicateName())
	}
	if s.TopReactions(0, 0).GetPredicateName() != mtproto.Predicate_messages_reactions {
		t.Fatal("hash 0 must return the list")
	}

	const user int64 = 9
	s.NoteReaction(user, []string{def[len(def)-1], def[len(def)-1]})
	top = s.TopReactions(1, 0)
	if len(top.GetReactions()) != 1 || top.GetReactions()[0].GetEmoticon() != def[len(def)-1] {
		t.Fatalf("top after use %#v", top.GetReactions())
	}
	if s.reactionUses[def[len(def)-1]] != 2 {
		t.Fatalf("count %d", s.reactionUses[def[len(def)-1]])
	}

	recent := s.RecentReactions(user, 10, 0)
	if len(recent.GetReactions()) != 1 || recent.GetReactions()[0].GetEmoticon() != def[len(def)-1] {
		t.Fatalf("recent %#v", recent.GetReactions())
	}
	if again := s.RecentReactions(user, 10, recent.GetHash()); again.GetPredicateName() != mtproto.Predicate_messages_reactionsNotModified {
		t.Fatalf("recent hash %s", again.GetPredicateName())
	}
	if other := s.RecentReactions(user+1, 10, 0); len(other.GetReactions()) != 0 {
		t.Fatalf("other user recent %#v", other.GetReactions())
	}

	s.NoteReaction(user, []string{def[0]})
	recent = s.RecentReactions(user, 10, 0)
	if len(recent.GetReactions()) != 2 || recent.GetReactions()[0].GetEmoticon() != def[0] {
		t.Fatalf("recent order %#v", recent.GetReactions())
	}
	s.ClearRecentReactions(user)
	if len(s.RecentReactions(user, 10, 0).GetReactions()) != 0 {
		t.Fatal("clear did not empty recent reactions")
	}
	if s.reactionUses[def[0]] != 1 {
		t.Fatal("clear removed the usage count")
	}
}
