package mtproto

import "testing"

func TestUnsetChatReactionsAllowTheDefaultSet(t *testing.T) {
	got := FromChatReactions(ChatReactionsTypeNotDefined, nil)
	if got.GetPredicateName() != Predicate_chatReactionsAll || got.GetAllowCustom() {
		t.Fatalf("unset chat reactions %#v", got)
	}
	if FromChatReactions(ChatReactionsTypeNone, nil).GetPredicateName() != Predicate_chatReactionsNone {
		t.Fatal("explicit none was rewritten")
	}
}
