package readoutbox

import "testing"

func TestPrivateSelfReadDoesNotMarkOutbox(t *testing.T) {
	// The sender is looking at the private chat and reads up to their own message.
	own := Targets(1, []Message{{SenderID: 1, DialogMessageID: 10}})
	if len(own) != 0 {
		t.Fatalf("private self-read targets = %#v", own)
	}
}

func TestOutboxCursorNeverMovesBackward(t *testing.T) {
	if !Advance(0, 9) || !Advance(8, 9) {
		t.Fatal("a newer outbox id must advance the cursor")
	}
	if Advance(9, 9) || Advance(9, 4) || Advance(9, 0) || Advance(0, 0) {
		t.Fatal("read_outbox_max_id moved backwards or stayed on an empty id")
	}
}

func TestPrivateReadReceiptIgnoresReaderOutboxCursor(t *testing.T) {
	// User 2 reads user 1's message. That message is box id 10 in user 2's
	// inbox. User 2's own ReadOutboxMaxId is already 40 because user 1 read a
	// later message user 2 sent. Those cursors are not the same id space of
	// meaning: 10 is an incoming message, 40 is an outgoing one. The receipt
	// for user 1 must still be sent.
	const (
		reader       = int64(2)
		sender       = int64(1)
		inboxMaxID   = int32(10)
		readOutboxID = int32(40)
	)
	if inboxMaxID > readOutboxID {
		t.Fatal("fixture must be the case the old comparison suppressed")
	}
	if !NotifySender(reader, sender) {
		t.Fatal("1:1 read of the peer's message was suppressed")
	}
	if NotifySender(reader, reader) || NotifySender(reader, 0) || NotifySender(0, sender) {
		t.Fatal("a self-read or an unknown author is not a receipt")
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
