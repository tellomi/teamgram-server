package engine

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestInviteWritesTheServiceMessageIntoTheChat(t *testing.T) {
	var got *GroupEnd
	e := New(Options{OnGroup: func(ev GroupEnd) {
		cp := ev
		got = &cp
	}})
	if _, err := e.CreateGroup(CreateGroup{
		UserID: 1, PeerType: mtproto.PEER_CHAT, PeerID: 50, RandomID: 7,
	}); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	g := e.groups[e.byPeer[partKey(mtproto.PEER_CHAT, 50)]]
	id, access := g.id, g.access
	e.mu.Unlock()
	if _, err := e.Invite(1, id, access, []int64{2, 3}); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.PeerType != mtproto.PEER_CHAT || got.PeerID != 50 {
		t.Fatalf("service target %+v", got)
	}
	if got.Action.GetPredicateName() != mtproto.Predicate_messageActionInviteToGroupCall {
		t.Fatalf("action %s", got.Action.GetPredicateName())
	}
	users := map[int64]bool{}
	for _, id := range got.Action.GetUsers() {
		users[id] = true
	}
	if !users[2] || !users[3] {
		t.Fatalf("invited %v", got.Action.GetUsers())
	}
}
