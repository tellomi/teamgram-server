package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestSavedReactionSearchReachesPastUntaggedPage(t *testing.T) {
	chatbox.ResetShared()
	t.Cleanup(chatbox.ResetShared)
	const self int64 = 4
	ctx := context.Background()
	saved := mtproto.MakePeerUtil(mtproto.PEER_SELF, self)
	if _, err := chatbox.Shared().React(ctx, self, saved, 1, []string{"👍"}, nil, false); err != nil {
		t.Fatal(err)
	}
	byID := map[int32]*mtproto.MessageBox{}
	var newest []*mtproto.MessageBox
	for id := int32(20); id >= 11; id-- {
		box := reactionBox(id, "plain", self, self)
		byID[id] = box
		newest = append(newest, box)
	}
	tagged := reactionBox(1, "old thumb", self, self)
	byID[1] = tagged
	fake := &reactionMessages{byID: byID, newest: newest}
	c := reactionSearchCore(ctx, self, fake)
	got, err := c.MessagesSearch(&mtproto.TLMessagesSearch{
		Peer:   mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer(),
		Q:      "",
		Filter: emptySearchFilter(),
		SavedReaction: []*mtproto.Reaction{
			mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
		},
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fake.searchCalls != 0 {
		t.Fatalf("search paged the untagged history %d times", fake.searchCalls)
	}
	if len(got.GetMessages()) != 1 || got.GetMessages()[0].GetId() != 1 || got.GetMessages()[0].GetMessage() != "old thumb" {
		t.Fatalf("messages %#v", got.GetMessages())
	}
}

func TestEmptyQueryOutsideSavedTagsIsEmpty(t *testing.T) {
	c := reactionSearchCore(context.Background(), 4, &reactionMessages{})
	_, err := c.MessagesSearch(&mtproto.TLMessagesSearch{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer(),
		Q:      "",
		Filter: emptySearchFilter(),
		Limit:  5,
	})
	if status.Convert(err).Message() != "SEARCH_QUERY_EMPTY" {
		t.Fatal(err)
	}
}

func TestDialogReactionSearchSkipsUntagged(t *testing.T) {
	chatbox.ResetShared()
	t.Cleanup(chatbox.ResetShared)
	const self int64 = 4
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 8)
	if _, err := chatbox.Shared().React(ctx, self, peer, 9, []string{"👍"}, nil, false); err != nil {
		t.Fatal(err)
	}
	byID := map[int32]*mtproto.MessageBox{
		10: reactionBox(10, "hello plain", self, 8),
		9:  reactionBox(9, "hello tagged", self, 8),
	}
	fake := &reactionMessages{byID: byID, newest: []*mtproto.MessageBox{byID[10], byID[9]}}
	c := reactionSearchCore(ctx, self, fake)
	got, err := c.MessagesSearch(&mtproto.TLMessagesSearch{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer(),
		Q:      "hello",
		Filter: emptySearchFilter(),
		SavedReaction: []*mtproto.Reaction{
			mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
		},
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetMessages()) != 1 || got.GetMessages()[0].GetId() != 9 {
		t.Fatalf("messages %#v", got.GetMessages())
	}
	for _, m := range got.GetMessages() {
		if m.GetId() == 10 {
			t.Fatal("untagged message was returned")
		}
	}
}

func emptySearchFilter() *mtproto.MessagesFilter {
	return mtproto.MakeTLInputMessagesFilterEmpty(&mtproto.MessagesFilter{}).To_MessagesFilter()
}

func reactionSearchCore(ctx context.Context, user int64, messages messageclient.MessageClient) *MessagesCore {
	return &MessagesCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: user},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: messages}},
	}
}

func reactionBox(id int32, text string, owner, peer int64) *mtproto.MessageBox {
	return &mtproto.MessageBox{
		UserId:    owner,
		MessageId: id,
		PeerType:  mtproto.PEER_USER,
		PeerId:    peer,
		Message: mtproto.MakeTLMessage(&mtproto.Message{
			Id:      id,
			Message: text,
			PeerId:  mtproto.MakePeerUser(peer),
		}).To_Message(),
	}
}

type reactionMessages struct {
	messageclient.MessageClient
	byID        map[int32]*mtproto.MessageBox
	newest      []*mtproto.MessageBox
	searchCalls int
}

func (f *reactionMessages) MessageSearchV2(_ context.Context, in *message.TLMessageSearchV2) (*mtproto.MessageBoxList, error) {
	f.searchCalls++
	n := int(in.GetLimit())
	if n > len(f.newest) {
		n = len(f.newest)
	}
	if n < 0 {
		n = 0
	}
	return &mtproto.MessageBoxList{BoxList: append([]*mtproto.MessageBox(nil), f.newest[:n]...), Count: int32(len(f.newest))}, nil
}

func (f *reactionMessages) MessageGetUserMessageList(_ context.Context, in *message.TLMessageGetUserMessageList) (*message.Vector_MessageBox, error) {
	var datas []*mtproto.MessageBox
	for _, id := range in.GetIdList() {
		if box := f.byID[id]; box != nil {
			datas = append(datas, box)
		}
	}
	return &message.Vector_MessageBox{Datas: datas}, nil
}
