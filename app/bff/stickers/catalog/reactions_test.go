package catalog

import (
	"bytes"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAvailableReactionsUseTheirOwnArt(t *testing.T) {
	s := newService(builtinPacks())
	msg := s.Reactions(0)
	if len(msg.GetReactions()) == 0 {
		t.Fatal("no reactions")
	}
	thumbs, _ := readAsset("assets/twemoji/1f44d.webp")
	seen := map[string]bool{}
	for _, r := range msg.GetReactions() {
		emo := r.GetReaction()
		if emo == "" || seen[emo] {
			t.Fatalf("duplicate or empty reaction %q", emo)
		}
		seen[emo] = true
		icon, mime, ok := s.DocumentFile(r.GetStaticIcon().GetId())
		if !ok || mime != "image/webp" || len(icon) == 0 {
			t.Fatalf("%s static icon", emo)
		}
		if emo != "👍" && bytes.Equal(icon, thumbs) {
			t.Fatalf("%s static icon fell back to thumbs up", emo)
		}
		var prev []byte
		for _, doc := range []*mtproto.Document{
			r.GetAppearAnimation(), r.GetSelectAnimation(), r.GetActivateAnimation(),
			r.GetEffectAnimation(), r.GetAroundAnimation(), r.GetCenterIcon(),
		} {
			body, animMIME, ok := s.DocumentFile(doc.GetId())
			if !ok || animMIME != "application/x-tgsticker" || !tgsCanvas512(body) || bytes.Equal(body, fallbackTGS()) {
				t.Fatalf("%s animation is blank", emo)
			}
			if prev != nil && bytes.Equal(prev, body) {
				t.Fatalf("%s reuses one animation for two slots", emo)
			}
			prev = body
		}
	}
	if !seen["👍"] || !seen["🔥"] || !seen["❤️"] {
		t.Fatalf("missing defaults %#v", seen)
	}
	if _, ok := reactionAssets("0000"); ok {
		t.Fatal("missing code was treated as complete")
	}
}

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
