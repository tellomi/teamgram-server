package core

import "testing"

func TestPrivateInboxNotDeliveredWhenRecipientBlockedSender(t *testing.T) {
	if privateInboxDelivered(true) {
		t.Fatal("a message to someone who blocked the sender was delivered")
	}
	if !privateInboxDelivered(false) {
		t.Fatal("a message to someone who did not block the sender was dropped")
	}
}
