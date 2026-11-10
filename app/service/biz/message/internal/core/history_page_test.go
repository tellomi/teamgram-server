package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func boxes(ids ...int32) []*mtproto.MessageBox {
	out := make([]*mtproto.MessageBox, len(ids))
	for i, id := range ids {
		out[i] = &mtproto.MessageBox{MessageId: id}
	}
	return out
}

func idsOf(boxes []*mtproto.MessageBox) []int32 {
	out := make([]int32, len(boxes))
	for i, box := range boxes {
		out[i] = box.GetMessageId()
	}
	return out
}

func TestTrimNewestFirst(t *testing.T) {
	page := boxes(100, 99, 98, 97, 96, 95, 94, 93, 92, 91, 90, 89, 88, 87, 86)
	got := idsOf(trimNewestFirst(page, 0, 10))
	want := []int32{100, 99, 98, 97, 96, 95, 94, 93, 92, 91}
	if len(got) != len(want) {
		t.Fatalf("no offset %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("no offset %v", got)
		}
	}
	// offset_id already selected ids < 100. add_offset 5 skips five more.
	got = idsOf(trimNewestFirst(page, 5, 10))
	want = []int32{95, 94, 93, 92, 91, 90, 89, 88, 87, 86}
	if len(got) != len(want) {
		t.Fatalf("add_offset %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("add_offset %v", got)
		}
	}
	if trimNewestFirst(page, 100, 10) != nil {
		t.Fatal("add_offset past the end returned a page")
	}
}
