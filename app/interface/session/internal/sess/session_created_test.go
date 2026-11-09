package sess

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
)

// TestReconnectDoesNotReplayNewSessionCreated is the official session rule.
// new_session_created is sent when a session id is first used, and again
// only when a lower msg_id moves first_msg_id. Closing the TCP socket and
// opening it again with the same session id must not send it.
func TestReconnectDoesNotReplayNewSessionCreated(t *testing.T) {
	const (
		tempKey    = int64(88011)
		sessionID  = int64(42)
		otherID    = int64(43)
		serverSalt = int64(0x1122334455667788)
		pingID     = int64(7)
	)
	base := time.Now().Unix() << 32
	firstID := base | 16
	laterID := base | 20
	lowerID := base | 8

	manager := NewMainAuthWrapperManager(&dao.Dao{})
	m := NewMainAuthWrapper(tempKey, 0, mtproto.AuthStateNew, nil, 0, manager)
	defer m.Stop()
	m.tempAuth.authId = tempKey
	now := int32(time.Now().Unix())
	m.tempAuth.cacheSalt = mtproto.MakeTLFutureSalt(&mtproto.FutureSalt{
		ValidSince: now - 60,
		ValidUntil: now + 30*60,
		Salt:       serverSalt,
	})

	ctx := context.Background()
	conn := func(id int64) *connData {
		return &connData{
			authType:  mtproto.AuthKeyTypeTemp,
			authId:    tempKey,
			gatewayId: "gw",
			sessionId: id,
		}
	}
	m.onSessionNew(ctx, conn(sessionID))
	sess := m.tempAuth.sessions[sessionID]
	if sess == nil || sess.sessionState != kSessionStateNew {
		t.Fatal("first connect did not open a new session")
	}

	replies := httpPing(t, m, sessionID, serverSalt, firstID, pingID)
	if !hasNewSession(replies) {
		t.Fatalf("first message did not create the session: %#v", replies)
	}
	if sess.sessionState != kSessionStateCreated || sess.firstMsgId != firstID {
		t.Fatalf("after first message state %d first %d", sess.sessionState, sess.firstMsgId)
	}
	// The client has processed new_session_created. An unacked copy is
	// still in the outgoing queue and would be retransmitted, which is
	// not a new announcement.
	ackOutgoing(sess)

	closed := conn(sessionID)
	closed.closedAt = time.Now().Add(time.Second).UnixNano()
	m.onSessionClosed(ctx, closed)
	m.onSessionNew(ctx, conn(sessionID))
	if sess.sessionState != kSessionStateCreated || sess.firstMsgId != firstID {
		t.Fatalf("reconnect reset the session: state %d first %d", sess.sessionState, sess.firstMsgId)
	}

	again := httpPing(t, m, sessionID, serverSalt, laterID, pingID)
	if hasNewSession(again) {
		t.Fatalf("reconnect sent new_session_created again: %#v", again)
	}
	if !hasPong(again) {
		t.Fatalf("reconnect did not answer ping: %#v", again)
	}

	m.onSessionNew(ctx, conn(otherID))
	other := httpPing(t, m, otherID, serverSalt, laterID, pingID)
	if !hasNewSession(other) {
		t.Fatalf("a new session id was not announced: %#v", other)
	}

	ackOutgoing(sess)
	lower := httpPing(t, m, sessionID, serverSalt, lowerID, pingID)
	if !hasNewSession(lower) {
		t.Fatalf("a lower msg_id did not refresh first_msg_id: %#v", lower)
	}
	if sess.firstMsgId != lowerID {
		t.Fatalf("first_msg_id %d want %d", sess.firstMsgId, lowerID)
	}
}

func httpPing(t *testing.T, m *MainAuthWrapper, sessionID, salt, msgID, pingID int64) []mtproto.TLObject {
	t.Helper()
	// The first HTTP body is new_session_created when this session id is new,
	// and the pong otherwise. One channel is enough to see which one it was.
	ch := make(chan interface{}, 1)
	m.onSessionHttpData(context.Background(), &sessionHttpData{
		authType:   mtproto.AuthKeyTypeTemp,
		authId:     m.tempAuth.authId,
		gatewayId:  "gw",
		sessionId:  sessionID,
		salt:       salt,
		buf:        encodePing(msgID, pingID),
		resChannel: ch,
	})
	obj := readMaybe(t, ch)
	if obj == nil {
		t.Fatal("ping got no reply")
	}
	var replies []mtproto.TLObject
	collectReplies(obj, &replies)
	return replies
}

func readMaybe(t *testing.T, ch chan interface{}) mtproto.TLObject {
	t.Helper()
	select {
	case raw := <-ch:
		body, ok := raw.([]byte)
		if !ok || len(body) < 32 {
			t.Fatalf("gateway payload %#v", raw)
		}
		n := int(body[28]) | int(body[29])<<8 | int(body[30])<<16 | int(body[31])<<24
		if n <= 0 || 32+n > len(body) {
			t.Fatalf("body len %d payload %d", n, len(body))
		}
		obj := mtproto.NewDecodeBuf(body[32 : 32+n]).Object()
		if obj == nil {
			t.Fatalf("undecodable reply %x", body[32:32+n])
		}
		return obj
	default:
		return nil
	}
}

func hasNewSession(replies []mtproto.TLObject) bool {
	for _, obj := range replies {
		if _, ok := obj.(*mtproto.TLNewSessionCreated); ok {
			return true
		}
	}
	return false
}

func ackOutgoing(sess *session) {
	now := time.Now().Unix()
	for e := sess.outQueue.oMsgs.Front(); e != nil; e = e.Next() {
		e.Value.(*outboxMsg).sent = now
	}
}

func hasPong(replies []mtproto.TLObject) bool {
	for _, obj := range replies {
		if _, ok := obj.(*mtproto.TLPong); ok {
			return true
		}
	}
	return false
}
