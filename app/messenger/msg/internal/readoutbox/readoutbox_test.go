package readoutbox

import "testing"

func TestPrivateSelfReadDoesNotMarkOutbox(t *testing.T) {
	// The sender is looking at the private chat and reads up to their own message.
	own := Targets(1, []Message{{SenderID: 1, DialogMessageID: 10}})
	if len(own) != 0 {
		t.Fatalf("private self-read targets = %#v", own)
	}
}

func TestPrivateOtherReadMarksSender(t *testing.T) {
	got := Targets(2, []Message{
		{SenderID: 2, DialogMessageID: 4},
		{SenderID: 1, DialogMessageID: 9},
	})
	if len(got) != 1 || got[0].SenderID != 1 || got[0].DialogMessageID != 9 {
		t.Fatalf("private targets = %#v", got)
	}
}

func TestBasicGroupSelfReadDoesNotMarkOutbox(t *testing.T) {
	group := Targets(1, []Message{
		{SenderID: 2, DialogMessageID: 5},
		{SenderID: 1, DialogMessageID: 8},
		{SenderID: 1, DialogMessageID: 11},
	})
	if len(group) != 1 || group[0].SenderID != 2 || group[0].DialogMessageID != 5 {
		t.Fatalf("group self-read should only receipt the other author, got %#v", group)
	}
}

func TestBasicGroupOtherMemberMarksEveryAuthor(t *testing.T) {
	// Group of three. Member 3 reads messages from 1 and 2. Both outboxes move,
	// and an older id does not win over a newer one from the same sender.
	got := Targets(3, []Message{
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
