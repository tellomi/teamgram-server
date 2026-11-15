package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDateInWindowMatchesChannelSearch(t *testing.T) {
	early, mid, late := int32(1_700_000_000), int32(1_700_000_100), int32(1_700_000_200)
	if !dateInWindow(early, 0, mid) || dateInWindow(late, 0, mid) {
		t.Fatal("max_date")
	}
	if dateInWindow(early, mid, 0) || !dateInWindow(late, mid, 0) {
		t.Fatal("min_date")
	}
	list := mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{
		BoxList: []*mtproto.MessageBox{
			{MessageId: 1, Message: &mtproto.Message{Id: 1, Date: early, Message: "early"}},
			{MessageId: 2, Message: &mtproto.Message{Id: 2, Date: late, Message: "late"}},
		},
	}).To_MessageBoxList()
	got := boxesInDate(list, 0, mid)
	if len(got.GetBoxList()) != 1 || got.GetBoxList()[0].GetMessage().GetMessage() != "early" {
		t.Fatalf("%#v", got.GetBoxList())
	}
}
