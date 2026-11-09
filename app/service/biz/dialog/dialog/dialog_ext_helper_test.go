package dialog

import (
	"fmt"
	"sort"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDialogOrderDateThenPeer(t *testing.T) {
	list := sortedDialogs(sampleDialogs())
	want := []string{
		"pinned:2:1",
		"pinned:2:2",
		"4:7",
		"2:4",
		"2:3",
		"3:5",
		"2:9",
	}
	if got := dialogKeys(list); !sameKeys(got, want) {
		t.Fatalf("order %v", got)
	}

	again := sortedDialogs(reversed(sampleDialogs()))
	if got := dialogKeys(again); !sameKeys(got, want) {
		t.Fatalf("reshuffled %v", got)
	}
}

func TestPeerDialogsOmitEmptyRows(t *testing.T) {
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:     80,
		PeerId: mtproto.MakePeer(mtproto.PEER_USER, 9),
	}).To_Message()
	helper := &DialogsDataHelper{
		Dialogs: []*mtproto.Dialog{
			mtproto.MakeTLDialog(&mtproto.Dialog{
				Peer:       mtproto.MakePeer(mtproto.PEER_USER, 9),
				TopMessage: 80,
			}).To_Dialog(),
			mtproto.MakeTLDialog(&mtproto.Dialog{
				Peer:       mtproto.MakePeer(mtproto.PEER_USER, 4),
				TopMessage: 0,
			}).To_Dialog(),
			mtproto.MakeTLDialog(&mtproto.Dialog{
				Peer:       mtproto.MakePeer(mtproto.PEER_USER, 5),
				TopMessage: 50,
			}).To_Dialog(),
		},
		Messages: []*mtproto.Message{msg},
		Users: []*mtproto.User{
			mtproto.MakeTLUser(&mtproto.User{Id: 9}).To_User(),
			mtproto.MakeTLUser(&mtproto.User{Id: 4}).To_User(),
			mtproto.MakeTLUser(&mtproto.User{Id: 5}).To_User(),
		},
	}
	out := helper.ToMessagesPeerDialogs(nil)
	if len(out.GetDialogs()) != 1 || out.GetDialogs()[0].GetTopMessage() != 80 {
		t.Fatalf("dialogs %+v", out.GetDialogs())
	}
	if len(out.GetMessages()) != 1 || out.GetMessages()[0].GetId() != 80 {
		t.Fatalf("messages %+v", out.GetMessages())
	}
	if len(out.GetUsers()) != 1 || out.GetUsers()[0].GetId() != 9 {
		t.Fatalf("users %+v", out.GetUsers())
	}
}

func TestOffsetAtPositionZero(t *testing.T) {
	list := sortedDialogs(DialogExtList{
		testDialog(mtproto.PEER_USER, 1, 30, 1, false, 0),
		testDialog(mtproto.PEER_USER, 2, 20, 2, false, 0),
		testDialog(mtproto.PEER_USER, 3, 10, 3, false, 0),
	})
	page := list.GetDialogsByOffsetLimit(30, 1, peerUtil(mtproto.PEER_USER, 1), 10)
	if got, want := dialogKeys(page), []string{"2:2", "2:3"}; !sameKeys(got, want) {
		t.Fatalf("offset at 0 returned %v", got)
	}
}

func TestLastPageBoundary(t *testing.T) {
	list := sortedDialogs(DialogExtList{
		testDialog(mtproto.PEER_USER, 1, 50, 1, false, 0),
		testDialog(mtproto.PEER_USER, 2, 40, 2, false, 0),
		testDialog(mtproto.PEER_USER, 3, 30, 3, false, 0),
		testDialog(mtproto.PEER_USER, 4, 20, 4, false, 0),
		testDialog(mtproto.PEER_USER, 5, 10, 5, false, 0),
	})
	page := list.GetDialogsByOffsetLimit(20, 4, peerUtil(mtproto.PEER_USER, 4), 2)
	if got, want := dialogKeys(page), []string{"2:5"}; !sameKeys(got, want) {
		t.Fatalf("last page %v", got)
	}
	if tail := list.GetDialogsByOffsetLimit(10, 5, peerUtil(mtproto.PEER_USER, 5), 2); len(tail) != 0 {
		t.Fatalf("past the end %v", dialogKeys(tail))
	}
}

func TestEqualDatesPaginateWithoutDupOrGap(t *testing.T) {
	list := sortedDialogs(DialogExtList{
		testDialog(mtproto.PEER_CHANNEL, 7, 100, 70, false, 0),
		testDialog(mtproto.PEER_USER, 4, 100, 40, false, 0),
		testDialog(mtproto.PEER_CHAT, 5, 100, 50, false, 0),
		testDialog(mtproto.PEER_USER, 3, 100, 30, false, 0),
	})
	want := []string{"4:7", "3:5", "2:4", "2:3"}
	if got := dialogKeys(list); !sameKeys(got, want) {
		t.Fatalf("equal-date order %v", got)
	}
	for _, limit := range []int32{1, 2, 3} {
		got := dialogKeys(paginateAll(t, list, limit))
		if !sameKeys(got, want) {
			t.Fatalf("limit %d paginated %v", limit, got)
		}
	}
}

func TestRepeatedPaginationNoDupOrGap(t *testing.T) {
	list := sortedDialogs(sampleDialogs())
	want := dialogKeys(list)
	if list.Count() != int32(len(want)) {
		t.Fatalf("count %d", list.Count())
	}
	for _, limit := range []int32{1, 2, 3, 100} {
		got := dialogKeys(paginateAll(t, list, limit))
		if !sameKeys(got, want) {
			t.Fatalf("limit %d paginated %v", limit, got)
		}
	}
}

func TestMissingOffsetDoesNotRestart(t *testing.T) {
	list := sortedDialogs(sampleDialogs())
	page := list.GetDialogsByOffsetLimit(100, 99, peerUtil(mtproto.PEER_USER, 99), 10)
	got := dialogKeys(page)
	want := []string{"4:7", "2:4", "2:3", "3:5", "2:9"}
	if !sameKeys(got, want) {
		t.Fatalf("missing cursor returned %v", got)
	}
}

func TestCursorPeerMovedOlderIsNotDuplicated(t *testing.T) {
	list := sortedDialogs(DialogExtList{
		testDialog(mtproto.PEER_USER, 1, 200, 1, false, 0),
		testDialog(mtproto.PEER_USER, 2, 80, 2, false, 0),
		testDialog(mtproto.PEER_USER, 5, 50, 5, false, 0),
	})
	page := list.GetDialogsByOffsetLimit(100, 5, peerUtil(mtproto.PEER_USER, 5), 10)
	if got, want := dialogKeys(page), []string{"2:2"}; !sameKeys(got, want) {
		t.Fatalf("moved cursor %v", got)
	}
}

func TestSelfOffsetMatchesUserPeer(t *testing.T) {
	list := sortedDialogs(DialogExtList{
		testDialog(mtproto.PEER_USER, 8, 30, 8, false, 0),
		testDialog(mtproto.PEER_USER, 9, 10, 9, false, 0),
	})
	page := list.GetDialogsByOffsetLimit(30, 8, peerUtil(mtproto.PEER_SELF, 8), 10)
	if got, want := dialogKeys(page), []string{"2:9"}; !sameKeys(got, want) {
		t.Fatalf("self offset %v", got)
	}
}

func TestDialogsSliceCountIsTotal(t *testing.T) {
	h := &DialogsDataHelper{
		Dialogs: []*mtproto.Dialog{
			mtproto.MakeTLDialog(&mtproto.Dialog{
				Peer:       mtproto.MakePeer(mtproto.PEER_USER, 7),
				TopMessage: 1,
			}).To_Dialog(),
		},
		Messages: []*mtproto.Message{
			testMessage(mtproto.PEER_USER, 7, 1, 100),
		},
		Users: []*mtproto.User{
			mtproto.MakeTLUser(&mtproto.User{Id: 7}).To_User(),
		},
	}
	out := h.ToMessagesDialogs(9, false)
	if out.GetPredicateName() != mtproto.Predicate_messages_dialogsSlice {
		t.Fatalf("predicate %s", out.GetPredicateName())
	}
	if out.GetCount() != 9 {
		t.Fatalf("count %d", out.GetCount())
	}
	if len(out.GetDialogs()) != 1 {
		t.Fatalf("dialogs %d", len(out.GetDialogs()))
	}
}

func TestDialogsWhenTotalFitsInLimit(t *testing.T) {
	h := returnableHelper(2)
	out := h.ToMessagesDialogs(2, true)
	if out.GetPredicateName() != mtproto.Predicate_messages_dialogs {
		t.Fatalf("predicate %s", out.GetPredicateName())
	}
	if len(out.GetDialogs()) != 2 {
		t.Fatalf("dialogs %d", len(out.GetDialogs()))
	}
}

func TestOffsetPageIsSliceWithTotal(t *testing.T) {
	h := returnableHelper(1)
	out := h.ToMessagesDialogs(5, false)
	if out.GetPredicateName() != mtproto.Predicate_messages_dialogsSlice {
		t.Fatalf("predicate %s", out.GetPredicateName())
	}
	if out.GetCount() != 5 || len(out.GetDialogs()) != 1 {
		t.Fatalf("count %d dialogs %d", out.GetCount(), len(out.GetDialogs()))
	}
}

func TestOffsetEmptyPageKeepsTotalCount(t *testing.T) {
	out := (&DialogsDataHelper{}).ToMessagesDialogs(4, false)
	if out.GetPredicateName() != mtproto.Predicate_messages_dialogsSlice {
		t.Fatalf("predicate %s", out.GetPredicateName())
	}
	if out.GetCount() != 4 || len(out.GetDialogs()) != 0 {
		t.Fatalf("count %d dialogs %d", out.GetCount(), len(out.GetDialogs()))
	}
}

func TestFullDialogListOnlyWithoutOffset(t *testing.T) {
	empty := &mtproto.PeerUtil{PeerType: mtproto.PEER_EMPTY}
	if !FullDialogList(0, 0, empty, 3, 10) {
		t.Fatal("first page that fits should be messages.dialogs")
	}
	if !FullDialogList(0, 0, nil, 0, 100) {
		t.Fatal("empty folder on the first page should be messages.dialogs")
	}
	if FullDialogList(0, 0, empty, 11, 10) {
		t.Fatal("first page over the limit should be a slice")
	}
	peer := peerUtil(mtproto.PEER_USER, 5)
	if FullDialogList(20, 5, peer, 3, 100) {
		t.Fatal("offset page should stay a slice even when the total fits in limit")
	}
	if FullDialogList(20, 5, peer, 8, 100) {
		t.Fatal("offset page should stay a slice")
	}
	if FullDialogList(0, 5, empty, 1, 100) {
		t.Fatal("offset_id alone should stay a slice")
	}
}

func TestSliceWhenMoreDialogsRemain(t *testing.T) {
	h := returnableHelper(1)
	out := h.ToMessagesDialogs(5, false)
	if out.GetPredicateName() != mtproto.Predicate_messages_dialogsSlice {
		t.Fatalf("predicate %s", out.GetPredicateName())
	}
	if out.GetCount() != 5 {
		t.Fatalf("count %d", out.GetCount())
	}
}

func TestCountAfterFixDropsUnreturnable(t *testing.T) {
	h := returnableHelper(1)
	h.Dialogs = append(h.Dialogs, mtproto.MakeTLDialog(&mtproto.Dialog{
		Peer:       mtproto.MakePeer(mtproto.PEER_USER, 9),
		TopMessage: 0,
	}).To_Dialog())
	out := h.ToMessagesDialogs(2, false)
	if out.GetPredicateName() != mtproto.Predicate_messages_dialogsSlice {
		t.Fatalf("predicate %s", out.GetPredicateName())
	}
	if out.GetCount() != 1 || len(out.GetDialogs()) != 1 || out.GetDialogs()[0].GetTopMessage() != 1 {
		t.Fatalf("count %d dialogs %+v", out.GetCount(), out.GetDialogs())
	}
}

func TestTiedDatesPageByMessageID(t *testing.T) {
	list := sortedDialogs(DialogExtList{
		testDialog(mtproto.PEER_USER, 1, 100, 10, false, 0),
		testDialog(mtproto.PEER_USER, 2, 100, 30, false, 0),
		testDialog(mtproto.PEER_USER, 3, 100, 20, false, 0),
	})
	if got, want := dialogKeys(list), []string{"2:2", "2:3", "2:1"}; !sameKeys(got, want) {
		t.Fatalf("order %v", got)
	}
	page := list.GetDialogsByOffsetLimit(100, 30, peerUtil(mtproto.PEER_USER, 2), 10)
	if got, want := dialogKeys(page), []string{"2:3", "2:1"}; !sameKeys(got, want) {
		t.Fatalf("page %v", got)
	}
	if !list.PageIsTail(page, 10) {
		t.Fatal("remainder should fit in the page")
	}
	head := list.GetDialogsByOffsetLimit(0, 0, &mtproto.PeerUtil{PeerType: mtproto.PEER_EMPTY}, 1)
	if list.PageIsTail(head, 1) {
		t.Fatal("first row was treated as the tail")
	}
}

func TestMismatchedDate2UsesTopMessage(t *testing.T) {
	stale := testDialog(mtproto.PEER_USER, 1, 500, 11, false, 0)
	fresh := testDialog(mtproto.PEER_USER, 2, 10, 22, false, 0)
	list := DialogExtList{stale, fresh}
	list.ApplyTopMessages([]*mtproto.Message{
		testMessage(mtproto.PEER_USER, 1, 11, 20),
		testMessage(mtproto.PEER_USER, 2, 22, 40),
	})
	if got, want := dialogKeys(sortedDialogs(list)), []string{"2:2", "2:1"}; !sameKeys(got, want) {
		t.Fatalf("order %v", got)
	}
}

func TestDraftDateOutranksMessageDate(t *testing.T) {
	withDraft := testDialog(mtproto.PEER_USER, 1, 1, 11, false, 0)
	withDraft.Dialog.Draft = mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{
		Date_INT32: 90,
		Message:    "d",
	}).To_DraftMessage()
	other := testDialog(mtproto.PEER_USER, 2, 1, 22, false, 0)
	list := DialogExtList{withDraft, other}
	list.ApplyTopMessages([]*mtproto.Message{
		testMessage(mtproto.PEER_USER, 1, 11, 10),
		testMessage(mtproto.PEER_USER, 2, 22, 50),
	})
	if got, want := dialogKeys(sortedDialogs(list)), []string{"2:1", "2:2"}; !sameKeys(got, want) {
		t.Fatalf("order %v", got)
	}
}

func TestCursorDialogMovedUsesRequestCursor(t *testing.T) {
	moved := testDialog(mtproto.PEER_USER, 5, 200, 5, false, 0)
	middle := testDialog(mtproto.PEER_USER, 2, 80, 2, false, 0)
	newest := testDialog(mtproto.PEER_USER, 1, 200, 1, false, 0)
	list := DialogExtList{moved, middle, newest}
	list.ApplyTopMessages([]*mtproto.Message{
		testMessage(mtproto.PEER_USER, 5, 5, 50),
		testMessage(mtproto.PEER_USER, 2, 2, 80),
		testMessage(mtproto.PEER_USER, 1, 1, 200),
	})
	list = sortedDialogs(list)
	page := list.GetDialogsByOffsetLimit(100, 5, peerUtil(mtproto.PEER_USER, 5), 10)
	if got, want := dialogKeys(page), []string{"2:2"}; !sameKeys(got, want) {
		t.Fatalf("moved cursor %v", got)
	}
	if !list.PageIsTail(page, 10) {
		t.Fatal("last page was not the tail")
	}
}

func TestPinnedOrderIgnoresMessageDate(t *testing.T) {
	list := DialogExtList{
		testDialog(mtproto.PEER_USER, 1, 10, 1, true, 100),
		testDialog(mtproto.PEER_USER, 2, 900, 2, true, 50),
	}
	list.ApplyTopMessages([]*mtproto.Message{
		testMessage(mtproto.PEER_USER, 1, 1, 1),
		testMessage(mtproto.PEER_USER, 2, 2, 9999),
	})
	if got, want := dialogKeys(sortedDialogs(list)), []string{"pinned:2:1", "pinned:2:2"}; !sameKeys(got, want) {
		t.Fatalf("pinned order %v", got)
	}
}

func sampleDialogs() DialogExtList {
	return DialogExtList{
		testDialog(mtproto.PEER_USER, 4, 100, 40, false, 0),
		testDialog(mtproto.PEER_USER, 1, 10, 1, true, 300),
		testDialog(mtproto.PEER_CHAT, 5, 90, 50, false, 0),
		testDialog(mtproto.PEER_USER, 3, 100, 30, false, 0),
		testDialog(mtproto.PEER_USER, 2, 40, 2, true, 200),
		testDialog(mtproto.PEER_CHANNEL, 7, 100, 70, false, 0),
		testDialog(mtproto.PEER_USER, 9, 80, 90, false, 0),
	}
}

func returnableHelper(n int) *DialogsDataHelper {
	h := &DialogsDataHelper{}
	for i := 1; i <= n; i++ {
		id := int64(i)
		h.Dialogs = append(h.Dialogs, mtproto.MakeTLDialog(&mtproto.Dialog{
			Peer:       mtproto.MakePeer(mtproto.PEER_USER, id),
			TopMessage: int32(i),
		}).To_Dialog())
		h.Messages = append(h.Messages, testMessage(mtproto.PEER_USER, id, int32(i), 100))
		h.Users = append(h.Users, mtproto.MakeTLUser(&mtproto.User{Id: id}).To_User())
	}
	return h
}

func testMessage(peerType int32, peerID int64, id, date int32) *mtproto.Message {
	return mtproto.MakeTLMessage(&mtproto.Message{
		Id:     id,
		Date:   date,
		PeerId: mtproto.MakePeer(peerType, peerID),
	}).To_Message()
}

func testDialog(peerType int32, peerID, date int64, top int32, pinned bool, pinOrder int64) *DialogExt {
	order := date
	if pinned {
		order = pinOrder
	}
	return &DialogExt{
		Order: order,
		Date:  date,
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Pinned:     pinned,
			Peer:       mtproto.MakePeer(peerType, peerID),
			TopMessage: top,
		}).To_Dialog(),
	}
}

func peerUtil(peerType int32, peerID int64) *mtproto.PeerUtil {
	return &mtproto.PeerUtil{PeerType: peerType, PeerId: peerID}
}

func sortedDialogs(in DialogExtList) DialogExtList {
	out := append(DialogExtList(nil), in...)
	sort.Sort(sort.Reverse(out))
	return out
}

func reversed(in DialogExtList) DialogExtList {
	out := append(DialogExtList(nil), in...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func paginateAll(t *testing.T, list DialogExtList, limit int32) DialogExtList {
	t.Helper()
	var (
		date int32
		id   int32
		peer = &mtproto.PeerUtil{PeerType: mtproto.PEER_EMPTY}
		got  DialogExtList
	)
	for n := 0; n <= list.Len()+1; n++ {
		page := list.GetDialogsByOffsetLimit(date, id, peer, limit)
		if len(page) == 0 {
			return got
		}
		if int32(len(page)) > limit {
			t.Fatalf("page %d exceeds limit %d", len(page), limit)
		}
		got = append(got, page...)
		last := page[len(page)-1]
		typ, pid := dialogPeer(last)
		date = int32(dialogTopDate(last))
		id = last.GetDialog().GetTopMessage()
		peer = peerUtil(typ, pid)
	}
	t.Fatal("pagination did not finish")
	return nil
}

func dialogKeys(list DialogExtList) []string {
	out := make([]string, 0, len(list))
	for _, d := range list {
		typ, id := dialogPeer(d)
		if dialogPinned(d) {
			out = append(out, fmt.Sprintf("pinned:%d:%d", typ, id))
			continue
		}
		out = append(out, fmt.Sprintf("%d:%d", typ, id))
	}
	return out
}

func sameKeys(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
