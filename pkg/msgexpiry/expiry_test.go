package msgexpiry

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestExpiredLeavesUntimedRows(t *testing.T) {
	if Expired(100, 0, 1000) || Expired(0, 10, 1000) || Expired(100, 10, 0) {
		t.Fatal("untimed row expired")
	}
	if !Expired(100, 10, 110) || Expired(100, 10, 109) {
		t.Fatal("boundary")
	}
}

func TestFilterOffKeepsExpired(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	boxes := []*mtproto.MessageBox{{
		TtlPeriod: 10,
		Message:   &mtproto.Message{Date: 100, Message: "secret"},
	}}
	got := FilterBoxes(boxes, 1000)
	if len(got) != 1 || got[0].Message.Message != "secret" {
		t.Fatalf("%+v", got)
	}
}

func TestFilterOnHidesBothCopies(t *testing.T) {
	t.Setenv(EnvEnabled, "1")
	keep := &mtproto.MessageBox{Message: &mtproto.Message{Date: 100, Message: "stay"}}
	mine := &mtproto.MessageBox{TtlPeriod: 10, Message: &mtproto.Message{Date: 100, Message: "gone"}}
	theirs := &mtproto.MessageBox{Message: &mtproto.Message{Date: 100, TtlPeriod: wrapperspb.Int32(10), Message: "gone-too"}}
	got := FilterBoxes([]*mtproto.MessageBox{keep, mine, theirs, nil}, 1000)
	if len(got) != 1 || got[0].Message.Message != "stay" {
		t.Fatalf("%+v", got)
	}
	again := FilterBoxes([]*mtproto.MessageBox{keep}, 1000)
	if len(again) != 1 {
		t.Fatal("live row disappeared")
	}
}

func TestPlanIsIdempotent(t *testing.T) {
	items := []Item{
		{ID: 1, Date: 100, TTL: 10},
		{ID: 2, Date: 100, TTL: 0},
		{ID: 1, Date: 100, TTL: 10},
		{ID: 3, Date: 100, TTL: 10, Deleted: true},
	}
	first := Plan(items, 1000)
	if len(first) != 1 || first[0] != 1 {
		t.Fatalf("plan %v", first)
	}
	items[0].Deleted = true
	items[2].Deleted = true
	if again := Plan(items, 1000); len(again) != 0 {
		t.Fatalf("second plan %v", again)
	}
}
