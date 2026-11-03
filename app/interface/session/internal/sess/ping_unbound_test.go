package sess

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
)

// iOS sends ping with salt 0 on a new temp key before auth.bindTempAuthKey.
// The session has to answer: bad_server_salt, then new_session_created and pong
// once the client retries with that salt.
func TestPingFirstUnboundTempKeyGetsReplies(t *testing.T) {
	const (
		tempKey    = int64(88001)
		sessionID  = int64(42)
		serverSalt = int64(0x1122334455667788)
		pingID     = int64(7)
	)

	manager := NewMainAuthWrapperManager(&dao.Dao{})
	wrapper := NewMainAuthWrapper(tempKey, 0, mtproto.AuthStateNew, nil, 0, manager)
	defer wrapper.Stop()

	now := int32(time.Now().Unix())
	wrapper.tempAuth.authId = tempKey
	wrapper.tempAuth.cacheSalt = mtproto.MakeTLFutureSalt(&mtproto.FutureSalt{
		ValidSince: now - 60,
		ValidUntil: now + 30*60,
		Salt:       serverSalt,
	})

	ctx := context.Background()
	first := make(chan interface{}, 1)
	wrapper.onSessionHttpData(ctx, &sessionHttpData{
		authType:   mtproto.AuthKeyTypeTemp,
		authId:     tempKey,
		gatewayId:  "gw",
		sessionId:  sessionID,
		salt:       0,
		buf:        encodePing(clientMsgID(1), pingID),
		resChannel: first,
	})

	saltReply := readGatewayObject(t, first)
	bad, ok := saltReply.(*mtproto.TLBadServerSalt)
	if !ok {
		t.Fatalf("salt 0 reply %T", saltReply)
	}
	if bad.GetErrorCode() != kServerSaltIncorrect || bad.GetNewServerSalt() != serverSalt {
		t.Fatalf("bad_server_salt code %d salt %d", bad.GetErrorCode(), bad.GetNewServerSalt())
	}

	sess := wrapper.tempAuth.sessions[sessionID]
	if sess == nil {
		t.Fatal("ping did not open a session")
	}
	createdCh := make(chan interface{}, 1)
	sess.httpQueue.Push(createdCh)
	pongCh := make(chan interface{}, 1)
	wrapper.onSessionHttpData(ctx, &sessionHttpData{
		authType:   mtproto.AuthKeyTypeTemp,
		authId:     tempKey,
		gatewayId:  "gw",
		sessionId:  sessionID,
		salt:       serverSalt,
		buf:        encodePing(clientMsgID(2), pingID),
		resChannel: pongCh,
	})

	var replies []mtproto.TLObject
	collectReplies(readGatewayObject(t, createdCh), &replies)
	collectReplies(readGatewayObject(t, pongCh), &replies)

	var created *mtproto.TLNewSessionCreated
	var pong *mtproto.TLPong
	for _, obj := range replies {
		switch o := obj.(type) {
		case *mtproto.TLNewSessionCreated:
			created = o
		case *mtproto.TLPong:
			pong = o
		}
	}
	if created == nil || created.GetServerSalt() != serverSalt {
		t.Fatalf("new_session_created missing in %#v", replies)
	}
	if pong == nil || pong.GetPingId() != pingID {
		t.Fatalf("pong missing in %#v", replies)
	}
}

func collectReplies(obj mtproto.TLObject, out *[]mtproto.TLObject) {
	if c, ok := obj.(*mtproto.TLMsgContainer); ok {
		for _, m := range c.Messages {
			collectReplies(m.Object, out)
		}
		return
	}
	*out = append(*out, obj)
}

func clientMsgID(n int64) int64 {
	return (time.Now().Unix() << 32) | (n << 2)
}

func encodePing(msgID, pingID int64) []byte {
	x := mtproto.NewEncodeBuf(64)
	_ = (&mtproto.TLMessage2{
		MsgId: msgID,
		Seqno: 0,
		Object: &mtproto.TLPing{
			Constructor: mtproto.CRC32_ping,
			PingId:      pingID,
		},
	}).Encode(x, 0)
	return append([]byte(nil), x.GetBuf()...)
}

func readGatewayObject(t *testing.T, ch chan interface{}) mtproto.TLObject {
	t.Helper()
	select {
	case raw := <-ch:
		body, ok := raw.([]byte)
		if !ok || len(body) < 32 {
			t.Fatalf("gateway payload %#v", raw)
		}
		n := int(binary.LittleEndian.Uint32(body[28:32]))
		if n <= 0 || 32+n > len(body) {
			t.Fatalf("body len %d payload %d", n, len(body))
		}
		obj := mtproto.NewDecodeBuf(body[32 : 32+n]).Object()
		if obj == nil {
			t.Fatalf("undecodable reply %x", body[32:32+n])
		}
		return obj
	default:
		t.Fatal("unbound temp key ping got no reply")
	}
	return nil
}
