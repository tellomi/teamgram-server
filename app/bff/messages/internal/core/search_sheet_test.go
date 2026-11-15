package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/searchbox"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

func TestSearchMyMentionsSkipsUnmentioned(t *testing.T) {
	c := searchCore(4)
	testMediaRows = func(peerType int32, peerID int64, kind searchbox.MediaKind) ([]searchbox.Row, bool, error) {
		return []searchbox.Row{
			{ID: 3, Text: "hello", Message: &mtproto.Message{Id: 3, Message: "hello"}},
			{ID: 7, Text: "token hello", Mentioned: true, Message: &mtproto.Message{Id: 7, Message: "token hello"}},
			{ID: 8, Text: "other", Mentioned: true, Message: &mtproto.Message{Id: 8, Message: "other"}},
		}, false, nil
	}
	t.Cleanup(func() { testMediaRows = nil })
	peer := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 5}).To_InputPeer()
	filter := mtproto.MakeTLInputMessagesFilterMyMentions(&mtproto.MessagesFilter{}).To_MessagesFilter()
	got, err := c.MessagesSearch(&mtproto.TLMessagesSearch{
		Peer: peer, Q: "token", Filter: filter, Limit: 10,
	})
	if err != nil || len(got.GetMessages()) != 1 || got.GetMessages()[0].GetId() != 7 {
		t.Fatalf("%v %#v", err, got.GetMessages())
	}
}

func TestSearchSavedPeerFiltersSource(t *testing.T) {
	c := searchCore(4)
	fromB := mtproto.MakePeerUser(8)
	fromC := mtproto.MakePeerUser(9)
	testMediaRows = func(peerType int32, peerID int64, kind searchbox.MediaKind) ([]searchbox.Row, bool, error) {
		return []searchbox.Row{
			{ID: 2, Date: 20, Text: "token from c", Message: &mtproto.Message{Id: 2, Date: 20, Message: "token from c", SavedPeerId: fromC}},
			{ID: 1, Date: 10, Text: "token from b", Message: &mtproto.Message{Id: 1, Date: 10, Message: "token from b", SavedPeerId: fromB}},
		}, false, nil
	}
	t.Cleanup(func() { testMediaRows = nil })
	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	savedB := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer()
	got, err := c.MessagesSearch(&mtproto.TLMessagesSearch{
		Peer: peer, Q: "token", SavedPeerId: savedB, Limit: 10,
	})
	if err != nil || len(got.GetMessages()) != 1 || got.GetMessages()[0].GetId() != 1 {
		t.Fatalf("peer %v %#v", err, got.GetMessages())
	}
	emptyQ, err := c.MessagesSearch(&mtproto.TLMessagesSearch{
		Peer: peer, SavedPeerId: savedB, Limit: 10,
	})
	if err != nil || len(emptyQ.GetMessages()) != 1 || emptyQ.GetMessages()[0].GetId() != 1 {
		t.Fatalf("empty q %v %#v", err, emptyQ.GetMessages())
	}
}

func TestSavedReactionDropsUntagged(t *testing.T) {
	chatbox.ResetShared()
	t.Cleanup(chatbox.ResetShared)
	self := mtproto.MakePeerUtil(mtproto.PEER_SELF, 4)
	if _, err := chatbox.Shared().React(context.Background(), 4, self, 1, []string{"👍"}, nil, false); err != nil {
		t.Fatal(err)
	}
	c := searchCore(4)
	thumb := mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction()
	list := &mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{
		{MessageId: 1, Message: &mtproto.Message{Id: 1, Message: "token"}},
		{MessageId: 2, Message: &mtproto.Message{Id: 2, Message: "token"}},
	}}
	got := c.keepReactionBoxes(self, list, []*mtproto.Reaction{thumb})
	if got == nil || len(got.BoxList) != 1 || got.BoxList[0].GetMessageId() != 1 {
		t.Fatalf("%#v", got)
	}
}
