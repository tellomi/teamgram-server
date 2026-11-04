package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestFilterGlobalBoxesByKindAndDate(t *testing.T) {
	user := &mtproto.MessageBox{PeerType: mtproto.PEER_USER, PeerId: 1, Message: &mtproto.Message{Date: 100, Message: "a"}}
	group := &mtproto.MessageBox{PeerType: mtproto.PEER_CHAT, PeerId: 2, Message: &mtproto.Message{Date: 200, Message: "b"}}
	news := &mtproto.MessageBox{PeerType: mtproto.PEER_CHANNEL, PeerId: 3, Message: &mtproto.Message{Date: 150, Message: "c"}}
	mega := &mtproto.MessageBox{PeerType: mtproto.PEER_CHANNEL, PeerId: 4, Message: &mtproto.Message{Date: 50, Message: "d"}}
	list := &mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{user, group, news, mega}}
	kinds := map[int64]channelKind{
		3: {Broadcast: true},
		4: {Megagroup: true},
	}
	got := filterGlobalBoxes(list, globalQuery{BroadcastsOnly: true, MinDate: 100}, kinds, 10)
	if len(got.BoxList) != 1 || got.BoxList[0].PeerId != 3 {
		t.Fatalf("broadcasts %#v", got.BoxList)
	}

	list = &mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{user, group, news, mega}}
	got = filterGlobalBoxes(list, globalQuery{GroupsOnly: true, UsersOnly: true}, kinds, 1)
	if len(got.BoxList) != 1 || got.BoxList[0].PeerType == mtproto.PEER_CHANNEL {
		t.Fatalf("limit %#v", got.BoxList)
	}
}
