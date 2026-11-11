package historyttl

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	ttl "github.com/teamgram/teamgram-server/pkg/historyttl"
)

func TestDialogIDsPrivateChatIsOrderIndependent(t *testing.T) {
	a, b := int64(717), int64(727)
	d1, d2 := dialogIDs(a, mtproto.PEER_USER, b)
	e1, e2 := dialogIDs(b, mtproto.PEER_USER, a)
	if d1 != e1 || d2 != e2 {
		t.Fatalf("dialog ids differ: %d,%d vs %d,%d", d1, d2, e1, e2)
	}
	if d1 != a || d2 != b {
		t.Fatalf("expected sorted pair %d,%d got %d,%d", a, b, d1, d2)
	}
}

func TestDialogIDsGroupUsesChatID(t *testing.T) {
	d1, d2 := dialogIDs(717, mtproto.PEER_CHAT, 42)
	if d2 != 42 || d1 >= 0 {
		t.Fatalf("group dialog ids = %d,%d", d1, d2)
	}
}

func TestDialogPeerPrefersDialogIDOverSelfPeerID(t *testing.T) {
	userID, other := int64(727), int64(717)
	d1, d2 := dialogIDs(userID, mtproto.PEER_USER, other)
	box := ttl.Box{UserID: userID, PeerType: mtproto.PEER_USER, PeerID: userID, SenderID: other}
	gotType, gotID := dialogPeer(userID, box, d1, d2)
	if gotType != mtproto.PEER_USER || gotID != other {
		t.Fatalf("dialogPeer = %d,%d want user %d", gotType, gotID, other)
	}
}

func TestDialogPeerFallsBackToSenderWhenDialogIDsMissing(t *testing.T) {
	userID, other := int64(727), int64(717)
	box := ttl.Box{UserID: userID, PeerType: mtproto.PEER_USER, PeerID: userID, SenderID: other}
	gotType, gotID := dialogPeer(userID, box, 0, 0)
	if gotType != mtproto.PEER_USER || gotID != other {
		t.Fatalf("fallback dialogPeer = %d,%d want user %d", gotType, gotID, other)
	}
}

func TestDialogPeerGroupUsesDialogB(t *testing.T) {
	d1, d2 := dialogIDs(717, mtproto.PEER_CHAT, 42)
	box := ttl.Box{UserID: 717, PeerType: mtproto.PEER_CHAT, PeerID: 42}
	gotType, gotID := dialogPeer(717, box, d1, d2)
	if gotType != mtproto.PEER_CHAT || gotID != 42 {
		t.Fatalf("group dialogPeer = %d,%d", gotType, gotID)
	}
}

func TestPeriodAtDoesNotStampMessagesBeforeTheTimer(t *testing.T) {
	actions := []ttl.Action{{Date: 200, Period: 60}}
	if ttl.PeriodAt(actions, 199) != 0 {
		t.Fatal("messages sent before the timer must stay unstamped")
	}
	if ttl.PeriodAt(actions, 200) != 60 {
		t.Fatal("the set-timer message and later messages take the new period")
	}
}
