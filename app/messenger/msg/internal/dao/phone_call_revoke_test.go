package dao

import (
	"fmt"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
)

func TestCallLogPeerGroupsEachOtherParty(t *testing.T) {
	rows := []*dataobject.MessagesDO{
		{PeerType: mtproto.PEER_USER, PeerId: 8, DialogMessageId: 10},
		{PeerType: mtproto.PEER_USER, PeerId: 8, DialogMessageId: 11},
		{PeerType: mtproto.PEER_USER, PeerId: 9, DialogMessageId: 12},
		{DialogId1: 1, DialogId2: 4, DialogMessageId: 13},
		{DialogId1: -int64(mtproto.PEER_CHAT), DialogId2: 70, DialogMessageId: 14},
	}
	got := map[string][]int64{}
	for _, row := range rows {
		pt, pid := callLogPeer(1, row)
		got[fmt.Sprintf("%d:%d", pt, pid)] = append(got[fmt.Sprintf("%d:%d", pt, pid)], row.DialogMessageId)
	}
	if len(got["2:8"]) != 2 || got["2:8"][0] != 10 || got["2:8"][1] != 11 {
		t.Fatalf("user 8 %#v", got)
	}
	if len(got["2:9"]) != 1 || got["2:9"][0] != 12 {
		t.Fatalf("user 9 %#v", got)
	}
	pt, pid := callLogPeer(1, rows[3])
	if pt != mtproto.PEER_USER || pid != 4 {
		t.Fatalf("dialog peer %d %d", pt, pid)
	}
	pt, pid = callLogPeer(1, rows[4])
	if pt != mtproto.PEER_CHAT || pid != 70 {
		t.Fatalf("chat peer %d %d", pt, pid)
	}
}
