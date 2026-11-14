package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"google.golang.org/grpc/status"
)

func TestPeerFromUsernameRowHonorsActive(t *testing.T) {
	active, err := peerFromUsernameRow(&dataobject.UsernameDO{
		Username: "roomname",
		PeerType: mtproto.PEER_CHANNEL,
		PeerId:   9,
		Active:   true,
	})
	if err != nil || active.GetChannelId() != 9 {
		t.Fatalf("%+v %v", active, err)
	}
	if _, err := peerFromUsernameRow(&dataobject.UsernameDO{
		Username: "roomname",
		PeerType: mtproto.PEER_CHANNEL,
		PeerId:   9,
		Active:   false,
	}); status.Convert(err).Message() != "USERNAME_NOT_OCCUPIED" {
		t.Fatalf("inactive: %v", err)
	}
	if _, err := peerFromUsernameRow(&dataobject.UsernameDO{
		Username: "roomname",
		PeerType: mtproto.PEER_CHANNEL,
		PeerId:   9,
		Active:   true,
		Deleted:  true,
	}); status.Convert(err).Message() != "USERNAME_NOT_OCCUPIED" {
		t.Fatalf("deleted: %v", err)
	}
}
