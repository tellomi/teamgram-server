package sess

import (
	"bytes"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// iOS dictation sends messages.transcribeDraft inside invokeWithLayer and,
// when the socket has more than one message, a msg_container. The session
// drops the whole RPC when Object() cannot resolve 0x7e11da47.
func TestTranscribeDraftDecodesLikeTheSession(t *testing.T) {
	audio := []byte("OggS\x00opus-dictation")

	t.Run("bare", func(t *testing.T) {
		msg := decodeSessionMessage(t, encodeSessionMessage(t, 0, 1, draftTranscribe(t, audio)))
		req := transcribeRequest(t, sessionQuery(t, msg))
		if !bytes.Equal(req.DraftAudio(), audio) {
			t.Fatalf("audio %q", req.DraftAudio())
		}
		assertTranscribeRoute(t, req)
	})

	t.Run("container", func(t *testing.T) {
		innerID := clientMsgID(4)
		outerID := innerID + 4
		wrapped := encodeSessionMessage(t, outerID, 1, &mtproto.TLMsgContainer{
			Messages: []*mtproto.TLMessage2{{
				MsgId: innerID,
				Seqno: 1,
				Object: &mtproto.TLInvokeWithLayer{
					Constructor: mtproto.CRC32_invokeWithLayer,
					Layer:       mtproto.Layer,
					Query:       initConnectionQuery(t, draftTranscribe(t, audio)),
				},
			}},
		})
		msg := decodeSessionMessage(t, wrapped)
		req := transcribeRequest(t, sessionQuery(t, msg))
		if req.GetConstructor() != mtproto.DraftTranscribeConstructor {
			t.Fatalf("constructor 0x%x", uint32(req.GetConstructor()))
		}
		if !bytes.Equal(req.DraftAudio(), audio) {
			t.Fatalf("audio %q", req.DraftAudio())
		}
		assertTranscribeRoute(t, req)
	})
}

func draftTranscribe(t *testing.T, audio []byte) *mtproto.TLMessagesTranscribeAudio {
	t.Helper()
	req := &mtproto.TLMessagesTranscribeAudio{}
	req.SetDraftAudio(audio)
	return req
}

func initConnectionQuery(t *testing.T, query mtproto.TLObject) []byte {
	t.Helper()
	body, err := encodeTL(query)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := encodeTL(&mtproto.TLInitConnection{
		Constructor:    mtproto.CRC32_initConnection_c1cd5ea9,
		ApiId:          10840,
		DeviceModel:    "iPhone",
		SystemVersion:  "14.0.4",
		AppVersion:     "14.0.4 (73)",
		SystemLangCode: "zh",
		LangPack:       "ios",
		LangCode:       "zh",
		Query:          body,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func encodeTL(obj mtproto.TLObject) ([]byte, error) {
	buf := mtproto.NewEncodeBuf(256)
	if err := obj.Encode(buf, mtproto.Layer); err != nil {
		return nil, err
	}
	return append([]byte(nil), buf.GetBuf()...), nil
}

func encodeSessionMessage(t *testing.T, msgID int64, seqno int32, obj mtproto.TLObject) []byte {
	t.Helper()
	raw, err := encodeTL(&mtproto.TLMessage2{MsgId: msgID, Seqno: seqno, Object: obj})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// decodeSessionMessage is MainAuthWrapper.onSessionData's decode.
func decodeSessionMessage(t *testing.T, raw []byte) *mtproto.TLMessage2 {
	t.Helper()
	msg := new(mtproto.TLMessage2)
	d := mtproto.GetDecodeBuf(raw)
	err := msg.Decode(d)
	mtproto.PutDecodeBuf(d)
	if err != nil {
		t.Fatalf("onSessionData decode: %v", err)
	}
	return msg
}

// sessionQuery walks msg_container, invokeWithLayer, and initConnection the
// way session.onSessionMessageData and session_invoke.go do.
func sessionQuery(t *testing.T, msg *mtproto.TLMessage2) mtproto.TLObject {
	t.Helper()
	msgs := []*mtproto.TLMessage2{msg}
	if box, ok := msg.Object.(*mtproto.TLMsgContainer); ok {
		msgs = box.Messages
	}
	if len(msgs) != 1 || msgs[0] == nil || msgs[0].Object == nil {
		t.Fatalf("session message %#v", msg)
	}
	obj := msgs[0].Object
	for {
		switch w := obj.(type) {
		case *mtproto.TLInvokeWithLayer:
			obj = mustDecodeObject(t, w.GetQuery())
		case *mtproto.TLInitConnection:
			obj = mustDecodeObject(t, w.GetQuery())
		case *mtproto.TLGzipPacked:
			obj = w.Obj
		default:
			return obj
		}
	}
}

func mustDecodeObject(t *testing.T, raw []byte) mtproto.TLObject {
	t.Helper()
	d := mtproto.NewDecodeBuf(raw)
	obj := d.Object()
	if err := d.GetError(); err != nil {
		t.Fatalf("%v", err)
	}
	if obj == nil {
		t.Fatal("nil object")
	}
	return obj
}

func transcribeRequest(t *testing.T, obj mtproto.TLObject) *mtproto.TLMessagesTranscribeAudio {
	t.Helper()
	req, ok := obj.(*mtproto.TLMessagesTranscribeAudio)
	if !ok {
		t.Fatalf("decoded %T, want messages.transcribeDraft", obj)
	}
	return req
}

func TestTellomiWireConstructorsResolve(t *testing.T) {
	// 2115054151 is 0x7e112a47, the miscomputed key that hid transcribeDraft.
	if mtproto.NewTLObjectByClassID(2115054151) != nil {
		t.Fatal("0x7e112a47 is registered")
	}
	ids := []uint32{
		uint32(mtproto.DraftTranscribeConstructor),
		0x0578ccaa, 0xa80a7f76, 0xda177181, 0xdc5213c2,
		0x5059dc73, 0x7e73b3a5, 0x652f4c71, 0x6be7187b,
		0xdb1a3503, 0xdcf754b9, 0x52577805, 0x6c4666e3,
		0x79cf2195, 0xecb62fa0, 0x7e76d7d3, 0x33b8e5eb,
		0xba7a1dba, 0xb436c1ff, 0x9f2958dc,
		0x695dd042, 0x83c586ae, 0x35a7c6ca,
		0x725bb4eb, 0x3cdaf8aa, 0xec2030ae, 0xc0a2ea9f, 0x4aa5f0fe,
		0x57df45a7, 0xf3010cba, 0x8ed043b1, 0x068509ac,
		0x95ace84f, 0x64b334e1, 0xebece66d, 0x3fb92b5b, 0x45c1d652,
		0x7a3f6dff, 0xaaeadce0, 0x698a9b1a, 0xf4599ef2, 0x812c2ae6,
	}
	for _, id := range ids {
		if mtproto.NewTLObjectByClassID(int32(id)) == nil {
			t.Errorf("constructor 0x%08x is not registered", id)
		}
	}
}

func assertTranscribeRoute(t *testing.T, req *mtproto.TLMessagesTranscribeAudio) {
	t.Helper()
	tup := mtproto.FindRPCContextTuple(req)
	if tup == nil || tup.Method != "/mtproto.RPCTranscription/messages_transcribeAudio" {
		t.Fatalf("handler route %#v", tup)
	}
}
