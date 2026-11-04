package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestGlobalSearchCursorAndFolder(t *testing.T) {
	older := &mtproto.MessageBox{PeerType: mtproto.PEER_USER, PeerId: 2, MessageId: 1, Message: &mtproto.Message{Id: 1, Date: 10}}
	same := &mtproto.MessageBox{PeerType: mtproto.PEER_USER, PeerId: 8, MessageId: 4, Message: &mtproto.Message{Id: 4, Date: 20}}
	newer := &mtproto.MessageBox{PeerType: mtproto.PEER_CHAT, PeerId: 3, MessageId: 9, Message: &mtproto.Message{Id: 9, Date: 30}}
	list := &mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{newer, same, older}}
	cur := globalCursor{Active: true, Rate: 20, PeerType: mtproto.PEER_USER, PeerID: 8, MsgID: 5}
	got := filterGlobalPage(list, cur, nil, false, 10)
	if len(got.BoxList) != 2 {
		t.Fatalf("cursor kept %#v", ids(got))
	}
	folder := map[string]bool{peerKey(mtproto.PEER_USER, 2): true}
	list = &mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{newer, same, older}}
	got = filterGlobalPage(list, globalCursor{}, folder, true, 10)
	if len(got.BoxList) != 1 || got.BoxList[0].PeerId != 2 {
		t.Fatalf("folder %#v", ids(got))
	}
}

func ids(list *mtproto.MessageBoxList) []int32 {
	var out []int32
	for _, box := range list.GetBoxList() {
		out = append(out, box.GetMessageId())
	}
	return out
}
