package searchbox

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func msg(id, date int32, text string) Row {
	return Row{
		ID: id, Date: date, Sender: 7, Text: text,
		Message: &mtproto.Message{Id: id, Date: date, Message: text},
	}
}

func TestCalendarDaysAndPartial(t *testing.T) {
	// Two UTC days: 2024-01-02 and 2024-01-01.
	rows := []Row{
		msg(30, 1704153600+100, "a"), // 2024-01-02
		msg(20, 1704153600+50, "b"),
		msg(10, 1704067200+10, "c"), // 2024-01-01
	}
	got := BuildCalendar(rows, 0, 0, false)
	if got.Inexact || got.Count != 3 || len(got.Periods) != 2 || len(got.Messages) != 2 {
		t.Fatalf("%#v", got)
	}
	if got.Periods[0].Count != 2 || got.Periods[0].MaxMsgID != 30 || got.Periods[0].MinMsgID != 20 {
		t.Fatalf("day %#v", got.Periods[0])
	}
	if got.MinMsgID != 10 || got.Messages[0].GetId() != 30 {
		t.Fatalf("min %d preview %d", got.MinMsgID, got.Messages[0].GetId())
	}
	page := BuildCalendar(rows, 25, 0, false)
	if page.Count != 2 || page.Periods[0].MaxMsgID != 20 {
		t.Fatalf("offset %#v", page)
	}
	if _, err := MediaFor(mtproto.MakeTLInputMessagesFilterEmpty(&mtproto.MessagesFilter{}).To_MessagesFilter()); err != mtproto.ErrFilterNotSupported {
		t.Fatal(err)
	}
	if _, err := MediaFor(mtproto.MakeTLInputMessagesFilterMyMentions(&mtproto.MessagesFilter{}).To_MessagesFilter()); err != mtproto.ErrFilterNotSupported {
		t.Fatal(err)
	}
	kind, err := MediaFor(mtproto.MakeTLInputMessagesFilterPhotoVideo(&mtproto.MessagesFilter{}).To_MessagesFilter())
	if err != nil || kind.Type != mtproto.MEDIA_PHOTOVIDEO || kind.Pinned {
		t.Fatalf("%#v %v", kind, err)
	}
}

func TestPositionsAndLiveAndSent(t *testing.T) {
	rows := []Row{msg(5, 50, "Photo cat"), msg(4, 40, "dog"), msg(3, 30, "cat"), msg(2, 20, "x"), msg(1, 10, "y")}
	count, pos := BuildPositions(rows, 0, 3)
	if count != 5 || len(pos) != 3 || pos[0].Offset != 0 || pos[0].MsgID != 5 || pos[2].Offset != 4 || pos[2].MsgID != 1 {
		t.Fatalf("count %d %#v", count, pos)
	}
	live := []Row{
		{ID: 9, Date: 100, Sender: 1, Live: true, Period: 50},
		{ID: 8, Date: 90, Sender: 1, Live: true, Period: 50},
		{ID: 7, Date: 80, Sender: 2, Live: true, Period: 5},
		{ID: 6, Date: 10, Sender: 3, Live: true, Period: liveForever},
	}
	got := PickLive(live, 100, 10)
	if len(got) != 2 || got[0].ID != 9 || got[1].ID != 6 {
		t.Fatalf("%#v", got)
	}
	sent := SentMedia(rows, "CAT", 1)
	if len(sent) != 1 || sent[0].ID != 5 {
		t.Fatalf("%#v", sent)
	}
	ids := MentionIDs([]Row{{ID: 4, TopID: 9}, {ID: 3, TopID: 2}, {ID: 9}}, 9)
	if len(ids) != 2 || ids[0] != 4 || ids[1] != 9 {
		t.Fatalf("%v", ids)
	}
	if HashIDs([]int32{1, 2}) == 0 {
		t.Fatal("hash")
	}
}
