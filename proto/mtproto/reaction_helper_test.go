package mtproto

import "testing"

func TestNormalizeReactionEmoji(t *testing.T) {
	if got := NormalizeReactionEmoji("❤️"); got != "❤" {
		t.Fatalf("heart %q", got)
	}
	if got := NormalizeReactionEmoji("❤"); got != "❤" {
		t.Fatalf("bare %q", got)
	}
	if got := NormalizeReactionEmoji("❤️‍🔥"); got != "❤️‍🔥" {
		t.Fatalf("zwj %q", got)
	}
	if got := NormalizeReactionEmoji("👍"); got != "👍" {
		t.Fatalf("other %q", got)
	}
}

func TestUnsetChatReactionsAllowTheDefaultSet(t *testing.T) {
	got := FromChatReactions(ChatReactionsTypeNotDefined, nil)
	if got.GetPredicateName() != Predicate_chatReactionsAll || got.GetAllowCustom() {
		t.Fatalf("unset chat reactions %#v", got)
	}
	if FromChatReactions(ChatReactionsTypeNone, nil).GetPredicateName() != Predicate_chatReactionsNone {
		t.Fatal("explicit none was rewritten")
	}
}
