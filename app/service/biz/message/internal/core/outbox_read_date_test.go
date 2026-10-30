package core

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestDatesFromHitsNeverEpoch(t *testing.T) {
	now := time.Now().Unix()
	hits := []readHit{
		{UserID: 20, Date: 0},
		{UserID: 21, Date: now},
		{UserID: 10, Date: now}, // the viewer
	}
	got := datesFromHits(hits, 10)
	if len(got) != 1 {
		t.Fatalf("dates = %#v, want the one real read", got)
	}
	if got[0].GetUserId() != 21 {
		t.Fatalf("user = %d", got[0].GetUserId())
	}
	if got[0].GetDate() == 0 {
		t.Fatal("getOutboxReadDate timestamp is 0")
	}
	if int64(got[0].GetDate()) != now {
		t.Fatalf("date = %d, want %d", got[0].GetDate(), now)
	}

	// A zero column with a real row write time is still a read time.
	fallback := datesFromHits([]readHit{{UserID: 20, Date: now}}, 0)
	if len(fallback) != 1 || fallback[0].GetDate() == 0 {
		t.Fatal("getOutboxReadDate timestamp is 0")
	}
}

func TestOutboxReadDateWireNotEpoch(t *testing.T) {
	now := int32(time.Now().Unix())
	encoded := mtproto.MakeTLOutboxReadDate(&mtproto.OutboxReadDate{Date: now}).To_OutboxReadDate()
	buf := mtproto.NewEncodeBuf(32)
	if err := encoded.Encode(buf, 221); err != nil {
		t.Fatal(err)
	}
	decoded := &mtproto.OutboxReadDate{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	if decoded.GetDate() == 0 || decoded.GetDate() != now {
		t.Fatalf("wire date %d, want %d", decoded.GetDate(), now)
	}
}
