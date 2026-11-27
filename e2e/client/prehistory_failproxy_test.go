package e2e

import "testing"

// TestFaultProxyInboxDownIsOnlyReadStateWrite checks the injector itself.
// It does not open MySQL and it does not speak MTProto.
func TestFaultProxyInboxDownIsOnlyReadStateWrite(t *testing.T) {
	inbox := &participantFaultProxy{}
	inbox.ArmReadWrite()
	fail, msg := inbox.failQuery("INSERT INTO channel_read_state (channel_id, user_id) VALUES (?,?)")
	if !fail || msg != "inbox down" {
		t.Fatalf("read-state insert fail=%v msg=%q", fail, msg)
	}
	fail, msg = inbox.failStatement(nil, stmtMeta{readWrite: true})
	if !fail || msg != "inbox down" {
		t.Fatalf("prepared read-state write fail=%v msg=%q", fail, msg)
	}
	logText := inbox.LogText()
	if !containsCount(logText, inboxDownLog, 2) {
		t.Fatalf("log=%q", logText)
	}

	other := &participantFaultProxy{}
	other.ArmFeatureWrite()
	fail, msg = other.failQuery("UPDATE channel_feature SET data=? WHERE channel_id=?")
	if !fail || msg == "inbox down" || containsCount(other.LogText(), inboxDownLog, 1) {
		t.Fatalf("feature write impersonated inbox down: fail=%v msg=%q log=%q", fail, msg, other.LogText())
	}
	other.ArmParticipantWrite(0)
	fail, msg = other.failQuery("INSERT INTO channel_participants (channel_id, user_id) VALUES (?,?)")
	if !fail || msg == "inbox down" || containsCount(other.LogText(), inboxDownLog, 1) {
		t.Fatalf("participant write impersonated inbox down: fail=%v msg=%q log=%q", fail, msg, other.LogText())
	}

	selectOnly := &participantFaultProxy{}
	selectOnly.ArmReadWrite()
	fail, msg = selectOnly.failQuery("SELECT read_inbox_max_id FROM channel_read_state WHERE channel_id=?")
	if fail || msg == "inbox down" || selectOnly.LogText() != "" {
		t.Fatalf("select was treated as the inbox write: fail=%v msg=%q log=%q", fail, msg, selectOnly.LogText())
	}
}

func containsCount(text, needle string, want int) bool {
	n := 0
	for i := 0; i+len(needle) <= len(text); i++ {
		if text[i:i+len(needle)] == needle {
			n++
		}
	}
	return n == want
}
