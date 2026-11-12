package mysql_dao

import "testing"

func TestHistoryDeleteWhere(t *testing.T) {
	clause, args := HistoryDeleteWhere(HistoryDeleteFilter{})
	if clause != " and deleted = 0" || len(args) != 0 {
		t.Fatalf("open filter %q %#v", clause, args)
	}

	clause, args = HistoryDeleteWhere(HistoryDeleteFilter{MaxID: 40, MinDate: 10, MaxDate: 20})
	want := " and deleted = 0 and user_message_box_id <= ? and date2 >= ? and date2 <= ?"
	if clause != want {
		t.Fatalf("clause %q", clause)
	}
	if len(args) != 3 || args[0] != int32(40) || args[1] != int32(10) || args[2] != int32(20) {
		t.Fatalf("args %#v", args)
	}

	clause, args = HistoryDeleteWhere(HistoryDeleteFilter{MinDate: 5})
	if clause != " and deleted = 0 and date2 >= ?" || len(args) != 1 || args[0] != int32(5) {
		t.Fatalf("min only %q %#v", clause, args)
	}
}
