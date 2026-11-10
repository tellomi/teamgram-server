package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// getDifference collapses every updateDeleteMessages onto the final pts and
// clears pts_count. The message ids stay, and pts is the difference cursor.
// A second delete is folded into that same update.
func TestMergeDeleteMessagesKeepsIDsAndFinalPts(t *testing.T) {
	m := newMergeUpdatesHelper()
	first := mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{
		Messages:  []int32{6},
		Pts_INT32: 12,
		PtsCount:  1,
	}).To_Update()
	second := mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{
		Messages:  []int32{7},
		Pts_INT32: 13,
		PtsCount:  1,
	}).To_Update()
	m.merge(first, 13)
	m.merge(second, 13)

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
	if up.GetPtsCount() != 0 {
		t.Fatalf("pts_count %d", up.GetPtsCount())
	}
}
