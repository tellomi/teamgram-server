package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/jsonx"
)

func TestFirstUnwrittenPtsFillsTheHole(t *testing.T) {
	if pts, ok := firstUnwrittenPts(4, 5); !ok || pts != 5 {
		t.Fatalf("tail hole pts=%d ok=%v", pts, ok)
	}
	if _, ok := firstUnwrittenPts(5, 5); ok {
		t.Fatal("a caught-up counter was treated as a hole")
	}
}

func TestChooseReplayPtsKeepsStored(t *testing.T) {
	pts, count := chooseReplayPts(4, 1, true, 9)
	if pts != 4 || count != 1 {
		t.Fatalf("stored pts=%d count=%d, current 9 must not replace it", pts, count)
	}
	pts, count = chooseReplayPts(0, 0, false, 9)
	if pts != 9 || count != 1 {
		t.Fatalf("missing stored pts fell back to %d/%d", pts, count)
	}
}

func TestUpdateCarriesBoxReadsStoredJSON(t *testing.T) {
	raw, err := jsonx.Marshal(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Pts_INT32: 4,
		PtsCount:  1,
		Message_MESSAGE: mtproto.MakeTLMessage(&mtproto.Message{
			Id:      12,
			Message: "kept",
		}).To_Message(),
	}).To_Update())
	if err != nil {
		t.Fatal(err)
	}
	if !updateCarriesBox(string(raw), 12) {
		t.Fatalf("stored update was not recognized: %s", raw)
	}
	if updateCarriesBox(string(raw), 1) {
		t.Fatal("prefix id matched the stored update")
	}
}

func TestUpdateCarriesBoxIgnoresPrefix(t *testing.T) {
	raw := `{"predicate_name":"updateNewMessage","message_MESSAGE":{"predicate_name":"message","id":12,"message":"kept"}}`
	if !updateCarriesBox(raw, 12) {
		t.Fatal("box 12 was not recognized")
	}
	if updateCarriesBox(raw, 1) || updateCarriesBox(raw, 123) {
		t.Fatal("a prefix of the box id matched")
	}
}

func TestPlanInboxReplay(t *testing.T) {
	first := planInboxReplay(false, false, false, false)
	if !first.Insert || !first.AddUnread || !first.PersistPts || !first.Push || !first.MoveTop {
		t.Fatalf("first insert %#v", first)
	}
	pending := planInboxReplay(true, false, true, true)
	if pending.Insert || !pending.AddUnread || pending.PersistPts || !pending.Push || !pending.MoveTop {
		t.Fatalf("pending unread %#v", pending)
	}
	live := planInboxReplay(true, false, true, false)
	if live.Insert || live.AddUnread || live.PersistPts || !live.Push || !live.MoveTop {
		t.Fatalf("applied replay %#v", live)
	}
	recoverPts := planInboxReplay(true, false, false, true)
	if !recoverPts.PersistPts || !recoverPts.AddUnread || recoverPts.Insert {
		t.Fatalf("pts recovery %#v", recoverPts)
	}
	deleted := planInboxReplay(true, true, true, true)
	if deleted.Insert || deleted.Push || deleted.MoveTop || deleted.AddUnread || deleted.PersistPts {
		t.Fatalf("deleted replay %#v", deleted)
	}
}
