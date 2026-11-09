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
		"2:3",
		"2:4",
		"4:7",
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
	want := []string{"2:3", "2:4", "3:5", "4:7"}
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
	want := []string{"4:7", "3:5", "2:9"}
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
	}
	out := h.ToMessagesDialogs(9)
	if out.GetPredicateName() != mtproto.Predicate_messages_dialogsSlice {
		t.Fatalf("predicate %s", out.GetPredicateName())
	}
	if out.GetCount() != 9 {
		t.Fatalf("count %d", out.GetCount())
	}
	if len(out.GetDialogs()) != 0 {
		t.Fatalf("dropped dialog leaked into the page: %d", len(out.GetDialogs()))
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
