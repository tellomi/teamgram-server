package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
)

func TestTellomiPrivateOnlyTheServiceAccount(t *testing.T) {
	user := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: tellomiai.AccountID}
	if !tellomiPrivate(9, user) {
		t.Fatal("a person messaging @tellomi should be eligible")
	}
	if tellomiPrivate(tellomiai.AccountID, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: 9}) {
		t.Fatal("the service account must not reply to itself")
	}
	if tellomiPrivate(9, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: 8}) {
		t.Fatal("other peers are not @tellomi")
	}
	if tellomiPrivate(9, &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: tellomiai.AccountID}) {
		t.Fatal("groups are not phase 1")
	}
	if tellomiPrivate(0, user) {
		t.Fatal("anonymous sender")
	}
}

func TestMaybeTellomiDefaultOff(t *testing.T) {
	if tellomiai.Enabled() {
		t.Fatal("TELLOMI_AI is on in this test")
	}
	maybeTellomi(context.Background(), 9, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: tellomiai.AccountID}, "hi", 1)
}
