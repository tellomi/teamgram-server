package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/jsonx"
)

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
	first := planInboxReplay(false, false, false)
	if !first.Insert || !first.AddUnread || !first.PersistPts || !first.Push || !first.MoveTop {
		t.Fatalf("first insert %#v", first)
	}
	live := planInboxReplay(true, false, true)
	if live.Insert || live.AddUnread || live.PersistPts || !live.Push || !live.MoveTop {
		t.Fatalf("live replay %#v", live)
	}
	recoverPts := planInboxReplay(true, false, false)
	if !recoverPts.PersistPts || recoverPts.AddUnread || recoverPts.Insert {
		t.Fatalf("pts recovery %#v", recoverPts)
	}
	if !recoverPts.Push || !recoverPts.MoveTop {
		t.Fatal("a live row with no pts row still has to be pushed and may move top")
	}
	deleted := planInboxReplay(true, true, true)
	if deleted.Insert || deleted.Push || deleted.MoveTop || deleted.AddUnread || deleted.PersistPts {
		t.Fatalf("deleted replay %#v", deleted)
	}
}
