package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/searchbox"
	"github.com/teamgram/teamgram-server/app/bff/messages/sendas"
	"github.com/teamgram/teamgram-server/pkg/notifyack"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func searchCore(user int64) *MessagesCore {
	return &MessagesCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: user},
	}
}

func photoFilter() *mtproto.MessagesFilter {
	return mtproto.MakeTLInputMessagesFilterPhotoVideo(&mtproto.MessagesFilter{}).To_MessagesFilter()
}

func TestCalendarPositionsAndSentMedia(t *testing.T) {
	c := searchCore(4)
	rows := []searchbox.Row{
		{ID: 30, Date: 1704153700, Sender: 4, Text: "cat", Message: &mtproto.Message{Id: 30, Date: 1704153700, Message: "cat"}},
		{ID: 10, Date: 1704067210, Sender: 4, Text: "dog", Message: &mtproto.Message{Id: 10, Date: 1704067210, Message: "dog"}},
	}
	testMediaRows = func(peerType int32, peerID int64, kind searchbox.MediaKind) ([]searchbox.Row, bool, error) {
		if peerType == 0 {
			return rows, false, nil
		}
		if kind.Type != mtproto.MEDIA_PHOTOVIDEO {
			t.Fatalf("kind %#v", kind)
		}
		return rows, false, nil
	}
	t.Cleanup(func() { testMediaRows = nil })

	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer()
	cal, err := c.MessagesGetSearchResultsCalendar(&mtproto.TLMessagesGetSearchResultsCalendar{
		Peer: peer, Filter: photoFilter(),
	})
	if err != nil || cal.GetCount() != 2 || len(cal.GetPeriods()) != 2 || cal.GetMessages()[0].GetId() != 30 {
		t.Fatalf("%v %#v", err, cal)
	}
	if _, err := c.MessagesGetSearchResultsCalendar(&mtproto.TLMessagesGetSearchResultsCalendar{
		Peer: peer, Filter: mtproto.MakeTLInputMessagesFilterEmpty(&mtproto.MessagesFilter{}).To_MessagesFilter(),
	}); status.Convert(err).Message() != "FILTER_NOT_SUPPORTED" {
		t.Fatal(err)
	}
	pos, err := c.MessagesGetSearchResultsPositions(&mtproto.TLMessagesGetSearchResultsPositions{
		Peer: peer, Filter: photoFilter(), Limit: 2,
	})
	if err != nil || pos.GetCount() != 2 || len(pos.GetPositions()) != 2 || pos.GetPositions()[0].GetMsgId() != 30 {
		t.Fatalf("%v %#v", err, pos)
	}
	sent, err := c.MessagesSearchSentMedia(&mtproto.TLMessagesSearchSentMedia{Q: "cat", Filter: photoFilter(), Limit: 10})
	if err != nil || len(sent.GetMessages()) != 1 || sent.GetMessages()[0].GetId() != 30 {
		t.Fatalf("%v %#v", err, sent)
	}
}

func TestLiveMentionsAckAndSendAs(t *testing.T) {
	c := searchCore(4)
	testMediaRows = func(peerType int32, peerID int64, kind searchbox.MediaKind) ([]searchbox.Row, bool, error) {
		return []searchbox.Row{
			{ID: 9, Date: int32(1_700_000_000), Sender: 8, Live: true, Period: 0x7fffffff, TopID: 3, Message: &mtproto.Message{Id: 9}},
			{ID: 3, Date: int32(1_700_000_000), Sender: 9, TopID: 3, Message: &mtproto.Message{Id: 3}},
		}, false, nil
	}
	t.Cleanup(func() { testMediaRows = nil })
	peer := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 5}).To_InputPeer()
	live, err := c.MessagesGetRecentLocations(&mtproto.TLMessagesGetRecentLocations{Peer: peer, Limit: 10})
	if err != nil || len(live.GetMessages()) != 1 || live.GetMessages()[0].GetId() != 9 {
		t.Fatalf("%v %#v", err, live)
	}
	again, err := c.MessagesGetRecentLocations(&mtproto.TLMessagesGetRecentLocations{
		Peer: peer, Limit: 10, Hash: searchbox.HashIDs([]int32{9}),
	})
	if err != nil || again.GetPredicateName() != mtproto.Predicate_messages_messagesNotModified {
		t.Fatalf("%v %#v", err, again)
	}

	read, err := c.MessagesReadMentions(&mtproto.TLMessagesReadMentions{
		Peer: peer, TopMsgId: wrapperspb.Int32(3),
	})
	if err != nil || read.GetPtsCount() != 0 || read.GetOffset() != 0 {
		t.Fatalf("%v %#v", err, read)
	}
	if _, err := c.MessagesReadMentions(&mtproto.TLMessagesReadMentions{
		Peer: peer, TopMsgId: wrapperspb.Int32(-1),
	}); status.Convert(err).Message() != "MSG_ID_INVALID" {
		t.Fatal(err)
	}

	book := notifyack.New(nil)
	notifyack.Install(book)
	t.Cleanup(func() { notifyack.Install(nil) })
	if err := book.Note(c.ctx, 4, []int32{2, 6}); err != nil {
		t.Fatal(err)
	}
	acks, err := c.MessagesReceivedMessages(&mtproto.TLMessagesReceivedMessages{MaxId: 2})
	if err != nil || len(acks.GetDatas()) != 1 || acks.GetDatas()[0].GetId() != 2 {
		t.Fatalf("%v %#v", err, acks)
	}

	sendas.Install(sendas.New(nil))
	t.Cleanup(func() { sendas.Install(nil) })
	group := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 5}).To_InputPeer()
	self := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	if _, err := c.MessagesSaveDefaultSendAs(&mtproto.TLMessagesSaveDefaultSendAs{Peer: group, SendAs: self}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.MessagesSaveDefaultSendAs(&mtproto.TLMessagesSaveDefaultSendAs{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer(),
		SendAs: self,
	}); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatal(err)
	}
	choice, ok := sendas.Shared().Get(c.ctx, 4, mtproto.PEER_CHAT, 5)
	if !ok || choice.ID != 4 || choice.Type != mtproto.PEER_USER {
		t.Fatalf("%#v %v", choice, ok)
	}
}
