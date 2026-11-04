package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPrivateReadOutboxUpdate(t *testing.T) {
	// Reader 7 has seen the sender's message 9. The sender's cursor was 4.
	got := privateReadOutboxUpdate(7, 4, 9, 3)
	if got == nil {
		t.Fatal("missing updateReadHistoryOutbox")
	}
	if got.GetPredicateName() != mtproto.Predicate_updateReadHistoryOutbox {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
	peer := got.GetPeer_PEER()
	if peer == nil || peer.GetUserId() != 7 {
		t.Fatalf("peer = %+v", peer)
	}
	if got.GetMaxId() != 9 || got.GetPts_INT32() != 3 || got.GetPtsCount() != 1 {
		t.Fatalf("update max=%d pts=%d count=%d", got.GetMaxId(), got.GetPts_INT32(), got.GetPtsCount())
	}

	// A later read already moved the cursor to 20. An older receipt must not
	// be pushed, or the sender's checkmarks go backwards.
	if privateReadOutboxUpdate(7, 20, 9, 4) != nil {
		t.Fatal("older read produced updateReadHistoryOutbox")
	}
	if privateReadOutboxUpdate(7, 9, 9, 4) != nil {
		t.Fatal("equal read produced updateReadHistoryOutbox")
	}
	if privateReadOutboxUpdate(0, 0, 9, 1) != nil {
		t.Fatal("missing reader produced an update")
	}
}
