package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestUnreadMarkUpdateClearsAsWellAsSets(t *testing.T) {
	peer := mtproto.MakeUserPeerUtil(42)
	on := unreadMarkUpdate(true, peer)
	if on.GetPredicateName() != mtproto.Predicate_updateDialogUnreadMark || !on.GetUnread() {
		t.Fatalf("set %#v", on)
	}
	if on.GetPeer_DIALOGPEER().GetPeer().GetUserId() != 42 {
		t.Fatalf("peer %#v", on.GetPeer_DIALOGPEER())
	}
	off := unreadMarkUpdate(false, peer)
	if off.GetUnread() {
		t.Fatal("clear still has the unread flag")
	}
	if off.GetPeer_DIALOGPEER().GetPeer().GetUserId() != 42 {
		t.Fatalf("clear peer %#v", off.GetPeer_DIALOGPEER())
	}
}
