package engine

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestConferenceMethodsRejectOrdinaryGroupCalls(t *testing.T) {
	e := New(Options{})
	if _, err := e.CreateGroup(CreateGroup{
		UserID: 1, PeerType: mtproto.PEER_CHAT, PeerID: 50, RandomID: 7,
	}); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	g := e.groups[e.byPeer[partKey(mtproto.PEER_CHAT, 50)]]
	id, access := g.id, g.access
	e.mu.Unlock()
	if _, err := e.DeleteConferenceParticipants(1, id, access, false, true, []int64{2}, []byte{1}); status.Convert(err).Message() != "GROUPCALL_INVALID" {
		t.Fatalf("delete: %v", err)
	}
	if _, err := e.BroadcastBlock(1, id, access, []byte{1, 2, 3}); status.Convert(err).Message() != "GROUPCALL_INVALID" {
		t.Fatalf("broadcast: %v", err)
	}

	up, err := e.CreateConference(CreateConference{UserID: 1, Auth: 2, RandomID: 9, Block: []byte{1}})
	if err != nil {
		t.Fatal(err)
	}
	var callID int64
	for _, u := range up.GetUpdates() {
		if c := u.GetCall_GROUPCALL(); c != nil && c.GetConference() {
			callID = c.GetId()
		}
	}
	if callID == 0 {
		t.Fatal("conference was not created")
	}
	e.mu.Lock()
	conf := e.groups[callID]
	confAccess := conf.access
	e.mu.Unlock()
	if _, err = e.BroadcastBlock(1, callID, confAccess, []byte{9}); err != nil {
		t.Fatal(err)
	}
	if _, err = e.DeleteConferenceParticipants(1, callID, confAccess, false, true, []int64{2}, []byte{9}); err != nil {
		t.Fatal(err)
	}
}
