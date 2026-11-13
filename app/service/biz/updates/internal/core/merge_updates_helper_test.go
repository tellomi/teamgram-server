package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Two deletes in one getDifference window fold into one update. The ids
// stay in order. pts is the later update's own pts, and pts_count is the
// sum, so a client at 11 can apply 11+2 == 13. The window cursor is not
// written onto the update, and pts_count is not cleared.
func TestMergeDeleteMessagesKeepsIDsAndFinalPts(t *testing.T) {
	m := newMergeUpdatesHelper()
	m.merge(deleteMessages([]int32{6}, 12, 1))
	m.merge(deleteMessages([]int32{7}, 13, 1))

	got := m.toUpdates()
	if len(got) != 1 {
		t.Fatalf("updates = %d", len(got))
	}
	up := got[0]
	if up.GetPredicateName() != mtproto.Predicate_updateDeleteMessages {
		t.Fatalf("predicate %s", up.GetPredicateName())
	}
	ids := up.GetMessages()
	if len(ids) != 2 || ids[0] != 6 || ids[1] != 7 {
		t.Fatalf("ids %v", ids)
	}
	if up.GetPts_INT32() != 13 {
		t.Fatalf("pts %d", up.GetPts_INT32())
	}
	if up.GetPtsCount() != 2 {
		t.Fatalf("pts_count %d", up.GetPtsCount())
	}
	if end := assertClientPtsWalk(t, 11, got); end != 13 {
		t.Fatalf("walk ended at %d", end)
	}
}

func TestMergeSinglePtsUpdatesKeepOriginalPts(t *testing.T) {
	peer := mtproto.MakePeerUser(7)
	cases := []struct {
		name string
		upd  *mtproto.Update
	}{
		{"delete", deleteMessages([]int32{6}, 12, 2)},
		{"edit", historyClearEdit(5, 7, 1)},
		{"read_inbox", readInbox(peer, 0, 9, 4, 6, 1)},
		{"read_outbox", readOutbox(peer, 9, 8, 3)},
		{"read_contents", readContents([]int32{4}, 15, 1)},
		{"webpage", mtproto.MakeTLUpdateWebPage(&mtproto.Update{Pts_INT32: 4, PtsCount: 1}).To_Update()},
		{"folder", mtproto.MakeTLUpdateFolderPeers(&mtproto.Update{Pts_INT32: 9, PtsCount: 1}).To_Update()},
		{"pinned", mtproto.MakeTLUpdatePinnedMessages(&mtproto.Update{Peer_PEER: peer, Messages: []int32{3}, Pts_INT32: 8, PtsCount: 2}).To_Update()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantPts := tc.upd.GetPts_INT32()
			wantCount := tc.upd.GetPtsCount()
			if wantCount <= 0 {
				t.Fatalf("fixture pts_count %d", wantCount)
			}
			m := newMergeUpdatesHelper()
			m.merge(tc.upd)
			got := m.toUpdates()
			if len(got) != 1 {
				t.Fatalf("updates %d", len(got))
			}
			if got[0].GetPts_INT32() != wantPts || got[0].GetPtsCount() != wantCount {
				t.Fatalf("pts %d count %d, want %d %d", got[0].GetPts_INT32(), got[0].GetPtsCount(), wantPts, wantCount)
			}
		})
	}
}

// just_clear persists a read and then the history-clear edit. A client
// whose local pts is still 5 must be able to apply both, and the read must
// keep pts 6 rather than the window's last pts.
func TestMergeJustClearHistoryWalksPts(t *testing.T) {
	m := newMergeUpdatesHelper()
	m.merge(readInbox(mtproto.MakePeerUser(8), 0, 5, 0, 6, 1))
	m.merge(historyClearEdit(5, 7, 1))

	got := m.toUpdates()
	if len(got) != 2 {
		t.Fatalf("updates %d", len(got))
	}
	if got[0].GetPredicateName() != mtproto.Predicate_updateReadHistoryInbox || got[0].GetPts_INT32() != 6 || got[0].GetPtsCount() != 1 {
		t.Fatalf("read pts=%d count=%d", got[0].GetPts_INT32(), got[0].GetPtsCount())
	}
	edit := got[1]
	if edit.GetPredicateName() != mtproto.Predicate_updateEditMessage || edit.GetPts_INT32() != 7 || edit.GetPtsCount() != 1 {
		t.Fatalf("edit pts=%d count=%d", edit.GetPts_INT32(), edit.GetPtsCount())
	}
	action := edit.GetMessage_MESSAGE().GetAction().GetPredicateName()
	if action != mtproto.Predicate_messageActionHistoryClear {
		t.Fatalf("action %s", action)
	}
	if end := assertClientPtsWalk(t, 5, got); end != 7 {
		t.Fatalf("walk ended at %d", end)
	}
}

// A delete on either side of a different pts update stays in place. Moving
// both deletes onto one update would skip or repeat the edit's pts.
func TestMergeDeletesAroundAnotherUpdateStayWalkable(t *testing.T) {
	m := newMergeUpdatesHelper()
	m.merge(deleteMessages([]int32{6}, 12, 1))
	m.merge(historyClearEdit(5, 13, 1))
	m.merge(deleteMessages([]int32{7}, 14, 1))

	got := m.toUpdates()
	if len(got) != 3 {
		t.Fatalf("updates %d", len(got))
	}
	if got[0].GetMessages()[0] != 6 || got[2].GetMessages()[0] != 7 {
		t.Fatalf("ids %v then %v", got[0].GetMessages(), got[2].GetMessages())
	}
	if end := assertClientPtsWalk(t, 11, got); end != 14 {
		t.Fatalf("walk ended at %d", end)
	}
}

// Two deletes whose pts skip a value are not one update. Summing the counts
// would still fail localPts + pts_count == pts for the hole between them.
func TestMergeDeletesWithPtsHoleStaySeparate(t *testing.T) {
	m := newMergeUpdatesHelper()
	m.merge(deleteMessages([]int32{6}, 12, 1))
	m.merge(deleteMessages([]int32{7}, 14, 1))

	got := m.toUpdates()
	if len(got) != 2 {
		t.Fatalf("updates %d", len(got))
	}
	if got[0].GetPts_INT32() != 12 || got[0].GetPtsCount() != 1 || got[1].GetPts_INT32() != 14 || got[1].GetPtsCount() != 1 {
		t.Fatalf("pts %d/%d then %d/%d", got[0].GetPts_INT32(), got[0].GetPtsCount(), got[1].GetPts_INT32(), got[1].GetPtsCount())
	}
}

func TestMergeReadHistoryInboxFoldsAdjacentAndKeepsMaxId(t *testing.T) {
	peer := mtproto.MakePeerUser(42)
	m := newMergeUpdatesHelper()
	m.merge(readInbox(peer, 0, 3, 4, 6, 1))
	m.merge(readInbox(peer, 0, 5, 1, 7, 1))

	got := m.toUpdates()
	if len(got) != 1 {
		t.Fatalf("updates %d", len(got))
	}
	up := got[0]
	if up.GetMaxId() != 5 || up.GetStillUnreadCount() != 1 || up.GetPts_INT32() != 7 || up.GetPtsCount() != 2 {
		t.Fatalf("max=%d unread=%d pts=%d count=%d", up.GetMaxId(), up.GetStillUnreadCount(), up.GetPts_INT32(), up.GetPtsCount())
	}
	if end := assertClientPtsWalk(t, 5, got); end != 7 {
		t.Fatalf("walk ended at %d", end)
	}

	// A later read with a smaller max_id must not move the cursor backwards.
	back := newMergeUpdatesHelper()
	back.merge(readInbox(peer, 0, 10, 4, 6, 1))
	back.merge(readInbox(peer, 0, 4, 2, 7, 1))
	folded := back.toUpdates()
	if len(folded) != 1 || folded[0].GetMaxId() != 10 || folded[0].GetPtsCount() != 2 {
		t.Fatalf("folded %#v", folded)
	}
}

// The first inbox entry used to be removed and never written back, so a
// later read of that same peer dropped the earlier pts.
func TestMergeReadHistoryInboxAtIndexZeroStaysWalkable(t *testing.T) {
	a := mtproto.MakePeerUser(1)
	b := mtproto.MakePeerUser(2)
	m := newMergeUpdatesHelper()
	m.merge(readInbox(a, 0, 10, 3, 6, 1))
	m.merge(readInbox(b, 0, 1, 0, 7, 1))
	m.merge(readInbox(a, 0, 11, 2, 8, 1))

	got := m.toUpdates()
	if len(got) != 3 {
		t.Fatalf("updates %d", len(got))
	}
	if got[0].GetMaxId() != 10 || got[2].GetMaxId() != 11 {
		t.Fatalf("max %d then %d", got[0].GetMaxId(), got[2].GetMaxId())
	}
	if end := assertClientPtsWalk(t, 5, got); end != 8 {
		t.Fatalf("walk ended at %d", end)
	}
}

func TestMergeReadHistoryOutboxFoldsAdjacent(t *testing.T) {
	peer := mtproto.MakePeerUser(7)
	m := newMergeUpdatesHelper()
	m.merge(readOutbox(peer, 2, 8, 1))
	m.merge(readOutbox(peer, 4, 9, 1))

	got := m.toUpdates()
	if len(got) != 1 {
		t.Fatalf("updates %d", len(got))
	}
	up := got[0]
	if up.GetPredicateName() != mtproto.Predicate_updateReadHistoryOutbox || up.GetMaxId() != 4 || up.GetPts_INT32() != 9 || up.GetPtsCount() != 2 {
		t.Fatalf("max=%d pts=%d count=%d", up.GetMaxId(), up.GetPts_INT32(), up.GetPtsCount())
	}
}

func TestMergeReadMessagesContentsSumsPtsCount(t *testing.T) {
	m := newMergeUpdatesHelper()
	m.merge(readContents([]int32{1}, 3, 1))
	m.merge(readContents([]int32{2}, 4, 1))

	got := m.toUpdates()
	if len(got) != 1 {
		t.Fatalf("updates %d", len(got))
	}
	ids := got[0].GetMessages()
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 || got[0].GetPts_INT32() != 4 || got[0].GetPtsCount() != 2 {
		t.Fatalf("ids %v pts %d count %d", ids, got[0].GetPts_INT32(), got[0].GetPtsCount())
	}
}

func TestMergeFolderPeersAndPinnedKeepEachPts(t *testing.T) {
	m := newMergeUpdatesHelper()
	m.merge(mtproto.MakeTLUpdateFolderPeers(&mtproto.Update{Pts_INT32: 5, PtsCount: 1}).To_Update())
	m.merge(mtproto.MakeTLUpdateFolderPeers(&mtproto.Update{Pts_INT32: 6, PtsCount: 1}).To_Update())
	m.merge(mtproto.MakeTLUpdatePinnedMessages(&mtproto.Update{Pts_INT32: 7, PtsCount: 1}).To_Update())
	m.merge(mtproto.MakeTLUpdatePinnedMessages(&mtproto.Update{Pts_INT32: 8, PtsCount: 1}).To_Update())

	got := m.toUpdates()
	if end := assertClientPtsWalk(t, 4, got); end != 8 {
		t.Fatalf("walk ended at %d", end)
	}
}

func TestMergeNewMessageStaysOutOfOtherUpdates(t *testing.T) {
	msg := mtproto.MakeTLMessage(&mtproto.Message{Id: 4, Message: "hi"}).To_Message()
	m := newMergeUpdatesHelper()
	m.merge(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Message_MESSAGE: msg,
		Pts_INT32:       11,
		PtsCount:        1,
	}).To_Update())
	if len(m.newMessages) != 1 || m.newMessages[0].GetId() != 4 {
		t.Fatalf("new messages %#v", m.newMessages)
	}
	if len(m.toUpdates()) != 0 {
		t.Fatal("updateNewMessage was copied into other_updates")
	}
}

func TestMergeReadHistoryInboxFoldersStaySeparate(t *testing.T) {
	peer := mtproto.MakePeerUser(7)
	m := newMergeUpdatesHelper()
	m.merge(readInbox(peer, 0, 3, 1, 6, 1))
	m.merge(readInbox(peer, 1, 4, 0, 7, 1))
	got := m.toUpdates()
	if len(got) != 2 {
		t.Fatalf("updates %d", len(got))
	}
	if got[0].GetFolderId().GetValue() != 0 || got[1].GetFolderId().GetValue() != 1 {
		t.Fatalf("folders %d %d", got[0].GetFolderId().GetValue(), got[1].GetFolderId().GetValue())
	}
	if end := assertClientPtsWalk(t, 5, got); end != 7 {
		t.Fatalf("walk ended at %d", end)
	}
}

func assertClientPtsWalk(t *testing.T, start int32, ups []*mtproto.Update) int32 {
	t.Helper()
	running := start
	for i, up := range ups {
		count := up.GetPtsCount()
		pts := up.GetPts_INT32()
		if count <= 0 {
			t.Fatalf("%s #%d pts_count=%d", up.GetPredicateName(), i, count)
		}
		if running+count != pts {
			t.Fatalf("%s #%d: %d+%d != %d", up.GetPredicateName(), i, running, count, pts)
		}
		running = pts
	}
	return running
}

func deleteMessages(ids []int32, pts, count int32) *mtproto.Update {
	return mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{
		Messages:  ids,
		Pts_INT32: pts,
		PtsCount:  count,
	}).To_Update()
}

func readContents(ids []int32, pts, count int32) *mtproto.Update {
	return mtproto.MakeTLUpdateReadMessagesContents(&mtproto.Update{
		Messages:  ids,
		Pts_INT32: pts,
		PtsCount:  count,
	}).To_Update()
}

func readInbox(peer *mtproto.Peer, folder, maxID, unread, pts, count int32) *mtproto.Update {
	return mtproto.MakeTLUpdateReadHistoryInbox(&mtproto.Update{
		FolderId:         &wrapperspb.Int32Value{Value: folder},
		Peer_PEER:        peer,
		MaxId:            maxID,
		StillUnreadCount: unread,
		Pts_INT32:        pts,
		PtsCount:         count,
	}).To_Update()
}

func readOutbox(peer *mtproto.Peer, maxID, pts, count int32) *mtproto.Update {
	return mtproto.MakeTLUpdateReadHistoryOutbox(&mtproto.Update{
		Peer_PEER: peer,
		MaxId:     maxID,
		Pts_INT32: pts,
		PtsCount:  count,
	}).To_Update()
}

func historyClearEdit(messageID, pts, count int32) *mtproto.Update {
	return mtproto.MakeTLUpdateEditMessage(&mtproto.Update{
		Message_MESSAGE: mtproto.MakeTLMessageService(&mtproto.Message{
			Id:     messageID,
			Action: mtproto.MakeMessageActionHistoryClear(),
		}).To_Message(),
		Pts_INT32: pts,
		PtsCount:  count,
	}).To_Update()
}
