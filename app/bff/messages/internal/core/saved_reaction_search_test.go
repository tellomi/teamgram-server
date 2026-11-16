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
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
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

func TestBasicGroupFromIDEmptyQueryReturnsThatSender(t *testing.T) {
	const (
		viewer int64 = 4
		sender int64 = 9
		other  int64 = 10
		chat   int64 = 77
	)
	fromSender := reactionBox(3, "from-sender", viewer, chat)
	fromSender.SenderUserId = sender
	fromSender.PeerType = mtproto.PEER_CHAT
	fromSender.Message.PeerId = nil
	fromOther := reactionBox(4, "from-other", viewer, chat)
	fromOther.SenderUserId = other
	fromOther.PeerType = mtproto.PEER_CHAT
	fromOther.Message.PeerId = nil
	fake := &reactionMessages{newest: []*mtproto.MessageBox{fromOther, fromSender}}
	c := reactionSearchCore(context.Background(), viewer, fake)
	c.svcCtx.Dao.UserClient = emptySearchUsers{}
	got, err := c.MessagesSearch(&mtproto.TLMessagesSearch{
		Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: chat}).To_InputPeer(),
		FromId: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: sender}).To_InputPeer(),
		Q:      "",
		Filter: emptySearchFilter(),
		Limit:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fake.searchCalls != 1 || fake.lastSearch.GetFromId() != sender || fake.lastSearch.GetQ() != "" || fake.lastSearch.GetPeerType() != mtproto.PEER_CHAT {
		t.Fatalf("search %#v calls=%d", fake.lastSearch, fake.searchCalls)
	}
	if len(got.GetMessages()) != 1 || got.GetMessages()[0].GetMessage() != "from-sender" {
		t.Fatalf("messages %#v", got.GetMessages())
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

type emptySearchUsers struct {
	userclient.UserClient
}

func (emptySearchUsers) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{}, nil
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
	lastSearch  *message.TLMessageSearchV2
}

func (f *reactionMessages) MessageSearchV2(_ context.Context, in *message.TLMessageSearchV2) (*mtproto.MessageBoxList, error) {
	f.searchCalls++
	f.lastSearch = in
	src := f.newest
	if in.GetFromId() != 0 {
		src = nil
		for _, box := range f.newest {
			if box.GetSenderUserId() == in.GetFromId() {
				src = append(src, box)
			}
		}
	}
	n := int(in.GetLimit())
	if n > len(src) {
		n = len(src)
	}
	if n < 0 {
		n = 0
	}
	return &mtproto.MessageBoxList{BoxList: append([]*mtproto.MessageBox(nil), src[:n]...), Count: int32(len(src))}, nil
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
