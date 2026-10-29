package readoutbox

import "testing"

func TestOwnReadDoesNotMarkOutbox(t *testing.T) {
	// Private chat and group: the sender reading their own message is not a reader.
	own := Targets(1, []Message{{SenderID: 1, DialogMessageID: 10}})
	if len(own) != 0 {
		t.Fatalf("own read targets = %#v", own)
	}
	group := Targets(1, []Message{
		{SenderID: 1, DialogMessageID: 8},
		{SenderID: 1, DialogMessageID: 11},
	})
	if len(group) != 0 {
		t.Fatalf("group self-read targets = %#v", group)
	}
}

func TestOtherMemberMarksSender(t *testing.T) {
	// Private chat: only the other person.
	got := Targets(2, []Message{
		{SenderID: 2, DialogMessageID: 4},
		{SenderID: 1, DialogMessageID: 9},
	})
	if len(got) != 1 || got[0].SenderID != 1 || got[0].DialogMessageID != 9 {
		t.Fatalf("private targets = %#v", got)
	}

	// Group of three. Member 3 reads messages from 1 and 2. Both outboxes move,
	// and an older id does not win over a newer one from the same sender.
	got = Targets(3, []Message{
		{SenderID: 1, DialogMessageID: 5},
		{SenderID: 2, DialogMessageID: 6},
		{SenderID: 1, DialogMessageID: 7},
		{SenderID: 3, DialogMessageID: 8},
	})
	if len(got) != 2 {
		t.Fatalf("group targets = %#v", got)
	}
	if got[0].SenderID != 1 || got[0].DialogMessageID != 7 {
		t.Fatalf("sender 1 = %#v", got[0])
	}
	if got[1].SenderID != 2 || got[1].DialogMessageID != 6 {
		t.Fatalf("sender 2 = %#v", got[1])
	}
}
