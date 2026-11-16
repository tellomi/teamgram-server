package sess

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

// A successful saveBigFilePart answer is a small Bool. If that packet is
// lost, the client repeats the same msg_id. The cached answer has to go
// out again. Re-running the handler would be a second write.
func TestDuplicateSaveBigFilePartResendsCachedBool(t *testing.T) {
	const (
		tempKey    = int64(88021)
		sessionID  = int64(77)
		serverSalt = int64(0x1122334455667788)
		reqMsgID   = int64(0) // filled below; must be divisible by 4
	)
	_ = reqMsgID

	manager := NewMainAuthWrapperManager(&dao.Dao{})
	wrapper := NewMainAuthWrapper(tempKey, 0, mtproto.AuthStateNormal, nil, 0, manager)
	defer wrapper.Stop()
	wrapper.client = &authsession.ClientSession{Layer: 214}
	wrapper.tempAuth.authId = tempKey
	now := int32(time.Now().Unix())
	wrapper.tempAuth.cacheSalt = mtproto.MakeTLFutureSalt(&mtproto.FutureSalt{
		ValidSince: now - 60,
		ValidUntil: now + 30*60,
		Salt:       serverSalt,
	})

	msgID := (time.Now().Unix() << 32) | 16
	sess := newSession(sessionID, wrapper.tempAuth)
	wrapper.tempAuth.sessions[sessionID] = sess
	sess.sessionState = kSessionStateCreated
	sess.firstMsgId = msgID
	sess.connState = kStateOnline
	sess.isHttp = true

	in := sess.inQueue.AddMsgId(msgID)
	in.state = RECEIVED | RESPONSE_GENERATED
	sess.sendRpcResultToQueue(context.Background(), "gw", msgID, mtproto.BoolTrue)
	cached := sess.outQueue.Lookup(msgID)
	if cached == nil || cached.msg == nil || cached.msg.Bytes == 0 || cached.msg.Bytes > detailedInfoThreshold {
		t.Fatalf("cached bool answer = %#v", cached)
	}
	// The first receipt was handed to the gateway and then lost.
	cached.sent = time.Now().Unix()
	answerID := cached.msg.MsgId
	before := sess.outQueue.oMsgs.Len()

	reply := make(chan interface{}, 1)
	sess.httpQueue.Push(reply)
	sess.onSessionMessageData(context.Background(), "gw", "127.0.0.1", serverSalt, &mtproto.TLMessage2{
		MsgId:  msgID,
		Seqno:  1,
		Object: &mtproto.TLUploadSaveBigFilePart{FileId: 42, FilePart: 514, FileTotalParts: 515, Bytes: []byte{1, 2, 3, 4}},
	})

	if in.state != RECEIVED|RESPONSE_GENERATED {
		t.Fatalf("duplicate re-entered the handler, state %#x", in.state)
	}
	if sess.outQueue.oMsgs.Len() != before {
		t.Fatalf("duplicate created another outgoing message: %d -> %d", before, sess.outQueue.oMsgs.Len())
	}
	if got := sess.outQueue.Lookup(msgID); got == nil || got.msg.MsgId != answerID {
		t.Fatal("cached answer was replaced")
	}

	select {
	case raw := <-reply:
		if len(raw.([]byte)) == 0 {
			t.Fatal("resent answer was empty")
		}
	case <-time.After(time.Second):
		t.Fatal("lost bool receipt was not resent for the same msg_id")
	}
}
