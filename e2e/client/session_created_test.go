package e2e

import (
	"testing"
	"time"
)

// TestNewSessionStillAnnouncedOnReconnect closes the socket and dials again
// with the same auth key. gotd allocates a new MTProto session id for that
// TCP connection, so the server must still send new_session_created and the
// account must keep answering. Reusing one session id across the disconnect
// is covered by TestReconnectDoesNotReplayNewSessionCreated: this client
// cannot pin a session id.
func TestNewSessionStillAnnouncedOnReconnect(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileIOS, "notice", "Notice")
	ctx, cancel := contextTimeout(60 * time.Second)
	defer cancel()
	if _, _, _, err := c.State(ctx); err != nil {
		t.Fatal(err)
	}
	sess, ok := c.Session()
	if !ok || sess.Key.Value.Zero() || sess.ID == 0 {
		t.Fatal("login did not produce a session")
	}
	c.Close()

	again, err := DialSession(ctx, cfg, ProfileIOS, sess)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again.Close)
	// new_session_created is the reply to the first content message, not to
	// the TCP handshake. updates.getState is that message.
	if _, _, _, err = again.State(ctx); err != nil {
		t.Fatalf("updates.getState after reconnect: %v", err)
	}
	restored, ok := again.Session()
	if !ok || restored.ID == 0 {
		t.Fatal("reconnect did not receive new_session_created")
	}
	if restored.ID == sess.ID {
		t.Fatal("reconnect reused the previous session id; this test would no longer prove a new session")
	}
}
