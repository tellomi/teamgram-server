package sess

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
)

func TestUnknownConstructorGetsRPCError(t *testing.T) {
	const (
		tempKey    = int64(88011)
		sessionID  = int64(9)
		serverSalt = int64(0x1122334455667788)
	)
	msgID := clientMsgID(6)
	wrapper := newSaltedWrapper(tempKey, serverSalt)
	defer wrapper.Stop()

	ch := make(chan interface{}, 1)
	wrapper.onSessionHttpData(context.Background(), &sessionHttpData{
		authType:   mtproto.AuthKeyTypeTemp,
		authId:     tempKey,
		gatewayId:  "gw",
		sessionId:  sessionID,
		salt:       serverSalt,
		buf:        encodeBareConstructor(msgID, 1, 0x00abcdef),
		resChannel: ch,
	})

	res := rpcResultOf(t, readGatewayObject(t, ch))
	if res.ReqMsgId != msgID {
		t.Fatalf("req_msg_id %d", res.ReqMsgId)
	}
	rpc, ok := res.Result.(*mtproto.TLRpcError)
	if !ok || rpc.GetErrorCode() != 400 || rpc.GetErrorMessage() != "INPUT_CONSTRUCTOR_INVALID" {
		t.Fatalf("reply %T %#v", res.Result, res.Result)
	}
}

func TestDecodeFailureBorrowsSaltOnlyForTheReply(t *testing.T) {
	const (
		tempKey   = int64(88014)
		sessionID = int64(12)
	)
	wrapper := NewMainAuthWrapper(tempKey, 0, mtproto.AuthStateNew, nil, 0, NewMainAuthWrapperManager(&dao.Dao{}))
	defer wrapper.Stop()
	wrapper.tempAuth.authId = tempKey

	ch := make(chan interface{}, 1)
	wrapper.onSessionHttpData(context.Background(), &sessionHttpData{
		authType:   mtproto.AuthKeyTypeTemp,
		authId:     tempKey,
		gatewayId:  "gw",
		sessionId:  sessionID,
		salt:       0x55,
		buf:        encodeBareConstructor(clientMsgID(12), 1, 0x00abcdef),
		resChannel: ch,
	})
	res := rpcResultOf(t, readGatewayObject(t, ch))
	if rpc, ok := res.Result.(*mtproto.TLRpcError); !ok || rpc.GetErrorMessage() != "INPUT_CONSTRUCTOR_INVALID" {
		t.Fatalf("reply %T %#v", res.Result, res.Result)
	}
	if wrapper.tempAuth.cacheSalt != nil {
		t.Fatal("decode failure stored a salt")
	}
}

func TestServiceMessageDecodeFailureStaysQuiet(t *testing.T) {
	const (
		tempKey    = int64(88012)
		sessionID  = int64(10)
		serverSalt = int64(0x1122334455667788)
	)
	wrapper := newSaltedWrapper(tempKey, serverSalt)
	defer wrapper.Stop()

	ch := make(chan interface{}, 1)
	wrapper.onSessionHttpData(context.Background(), &sessionHttpData{
		authType:   mtproto.AuthKeyTypeTemp,
		authId:     tempKey,
		gatewayId:  "gw",
		sessionId:  sessionID,
		salt:       serverSalt,
		buf:        encodeBareConstructor(clientMsgID(8), 0, 0x00abcdef),
		resChannel: ch,
	})
	select {
	case raw := <-ch:
		t.Fatalf("service message got a reply %#v", raw)
	default:
	}
}

func TestWrappedUnknownConstructorGetsRPCError(t *testing.T) {
	const (
		tempKey    = int64(88013)
		sessionID  = int64(11)
		serverSalt = int64(0x1122334455667788)
	)
	msgID := clientMsgID(10)
	wrapper := newSaltedWrapper(tempKey, serverSalt)
	defer wrapper.Stop()

	// Already created, so the first reply is the RPC error and not
	// new_session_created.
	sess := newSession(sessionID, wrapper.tempAuth)
	sess.sessionState = kSessionStateCreated
	sess.firstMsgId = 1
	wrapper.tempAuth.sessions[sessionID] = sess

	unknown := make([]byte, 4)
	binary.LittleEndian.PutUint32(unknown, 0x00abcdef)
	raw := encodeSessionMessage(t, msgID, 1, &mtproto.TLInvokeWithLayer{
		Constructor: mtproto.CRC32_invokeWithLayer,
		Layer:       mtproto.Layer,
		Query:       unknown,
	})

	ch := make(chan interface{}, 1)
	wrapper.onSessionHttpData(context.Background(), &sessionHttpData{
		authType:   mtproto.AuthKeyTypeTemp,
		authId:     tempKey,
		gatewayId:  "gw",
		sessionId:  sessionID,
		salt:       serverSalt,
		buf:        raw,
		resChannel: ch,
	})
	res := rpcResultOf(t, readGatewayObject(t, ch))
	if res.ReqMsgId != msgID {
		t.Fatalf("req_msg_id %d", res.ReqMsgId)
	}
	rpc, ok := res.Result.(*mtproto.TLRpcError)
	if !ok || rpc.GetErrorMessage() != "INPUT_CONSTRUCTOR_INVALID" {
		t.Fatalf("reply %T %#v", res.Result, res.Result)
	}
}

func newSaltedWrapper(authKeyID, salt int64) *MainAuthWrapper {
	wrapper := NewMainAuthWrapper(authKeyID, 0, mtproto.AuthStateNew, nil, 0, NewMainAuthWrapperManager(&dao.Dao{}))
	now := int32(time.Now().Unix())
	wrapper.tempAuth.authId = authKeyID
	wrapper.tempAuth.cacheSalt = mtproto.MakeTLFutureSalt(&mtproto.FutureSalt{
		ValidSince: now - 60,
		ValidUntil: now + 30*60,
		Salt:       salt,
	})
	return wrapper
}

func encodeBareConstructor(msgID int64, seqno int32, ctor uint32) []byte {
	body := make([]byte, 4)
	binary.LittleEndian.PutUint32(body, ctor)
	x := mtproto.NewEncodeBuf(32)
	x.Long(msgID)
	x.Int(seqno)
	x.Int(int32(len(body)))
	x.Bytes(body)
	return append([]byte(nil), x.GetBuf()...)
}

func rpcResultOf(t *testing.T, obj mtproto.TLObject) *mtproto.TLRpcResult {
	t.Helper()
	res, ok := obj.(*mtproto.TLRpcResult)
	if !ok || res == nil {
		t.Fatalf("reply %T", obj)
	}
	return res
}
