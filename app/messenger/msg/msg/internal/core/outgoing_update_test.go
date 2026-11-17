package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestOutgoingUpdateKeepsSenderOutAndRandomID(t *testing.T) {
	const randomID int64 = 0x1122334455667788
	body := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     false,
		Id:      3,
		FromId:  mtproto.MakePeerUser(10),
		PeerId:  mtproto.MakePeerUser(20),
		Message: "hello",
	}).To_Message()
	box := &mtproto.MessageBox{
		MessageId: 9,
		RandomId:  randomID,
		Pts:       15,
		PtsCount:  1,
		Message:   body,
	}
	up := outgoingUpdate(box)
	if body.GetOut() || body.GetId() != 3 {
		t.Fatalf("source message mutated: out=%v id=%d", body.GetOut(), body.GetId())
	}
	msg := up.GetMessage_MESSAGE()
	if !msg.GetOut() || msg.GetId() != 9 || msg.GetAction() != nil || up.GetRandomId() != randomID || up.GetPts_INT32() != 15 || up.GetPtsCount() != 1 {
		t.Fatalf("update = id=%d out=%v action=%v random=%d pts=%d count=%d", msg.GetId(), msg.GetOut(), msg.GetAction(), up.GetRandomId(), up.GetPts_INT32(), up.GetPtsCount())
	}

	reply := mtproto.MakeReplyUpdates(nil, nil, nil, up)
	var sawID, sawNew bool
	for _, u := range reply.GetUpdates() {
		switch u.GetPredicateName() {
		case mtproto.Predicate_updateMessageID:
			if sawNew {
				t.Fatal("updateMessageID arrived after updateNewMessage")
			}
			sawID = true
			if u.GetRandomId() != randomID || u.GetId_INT32() != 9 {
				t.Fatalf("updateMessageID random=%d id=%d", u.GetRandomId(), u.GetId_INT32())
			}
		case mtproto.Predicate_updateNewMessage:
			sawNew = true
			got := u.GetMessage_MESSAGE()
			if !got.GetOut() || got.GetId() != 9 || u.GetPtsCount() != 1 || u.GetPts_INT32() != 15 {
				t.Fatalf("updateNewMessage id=%d out=%v pts=%d count=%d", got.GetId(), got.GetOut(), u.GetPts_INT32(), u.GetPtsCount())
			}
		}
	}
	if !sawID || !sawNew {
		t.Fatalf("reply missing id=%v new=%v updates=%v", sawID, sawNew, reply.GetUpdates())
	}
}

func TestOutgoingUpdateFillsMissingPtsCount(t *testing.T) {
	up := outgoingUpdate(&mtproto.MessageBox{
		MessageId: 4,
		Pts:       8,
		Message:   mtproto.MakeTLMessage(&mtproto.Message{Message: "x"}).To_Message(),
	})
	if up.GetPtsCount() != 1 || up.GetPts_INT32() != 8 {
		t.Fatalf("pts=%d count=%d", up.GetPts_INT32(), up.GetPtsCount())
	}
}
