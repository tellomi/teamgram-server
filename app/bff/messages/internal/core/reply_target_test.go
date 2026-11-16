package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestGetMessagesReplyToReturnsTheOriginal(t *testing.T) {
	origin := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		Id:      206,
		Message: "origin",
		PeerId:  mtproto.MakePeerUser(8),
		FromId:  mtproto.MakePeerUser(7),
	}).To_Message()
	reply := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		Id:      207,
		Message: "reply",
		PeerId:  mtproto.MakePeerUser(8),
		FromId:  mtproto.MakePeerUser(7),
		ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
			ReplyToMsgId: 206,
		}).To_MessageReplyHeader(),
	}).To_Message()
	boxes := &replyBoxes{byID: map[int32]*mtproto.MessageBox{
		206: &mtproto.MessageBox{UserId: 7, MessageId: 206, SenderUserId: 7, PeerType: mtproto.PEER_USER, PeerId: 8, Message: origin},
		207: &mtproto.MessageBox{UserId: 7, MessageId: 207, SenderUserId: 7, PeerType: mtproto.PEER_USER, PeerId: 8, Message: reply},
	}}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, UserClient: replyUsers{}}})
	c.MD = &metadata.RpcMetadata{UserId: 7}
	got, err := c.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{
			mtproto.MakeTLInputMessageReplyTo(&mtproto.InputMessage{Id: 207}).To_InputMessage(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetMessages()) != 1 || got.GetMessages()[0].GetId() != 206 || got.GetMessages()[0].GetMessage() != "origin" {
		t.Fatalf("messages %#v", got.GetMessages())
	}
	flagged := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		Id:      208,
		Message: "flagged",
		PeerId:  mtproto.MakePeerUser(8),
		FromId:  mtproto.MakePeerUser(7),
		ReplyTo: &mtproto.MessageReplyHeader{
			PredicateName:          mtproto.Predicate_messageReplyHeader,
			ReplyToMsgId_FLAGINT32: &wrapperspb.Int32Value{Value: 206},
		},
	}).To_Message()
	boxes.byID[208] = &mtproto.MessageBox{UserId: 7, MessageId: 208, SenderUserId: 7, PeerType: mtproto.PEER_USER, PeerId: 8, Message: flagged}
	got, err = c.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{
			mtproto.MakeTLInputMessageReplyTo(&mtproto.InputMessage{Id: 208}).To_InputMessage(),
		},
	})
	if err != nil || len(got.GetMessages()) != 1 || got.GetMessages()[0].GetId() != 206 {
		t.Fatalf("flag form %#v %v", got.GetMessages(), err)
	}
}

type replyBoxes struct {
	messageclient.MessageClient
	byID map[int32]*mtproto.MessageBox
}

type replyUsers struct {
	userclient.UserClient
}

func (replyUsers) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{}, nil
}

func (f *replyBoxes) MessageGetUserMessageList(_ context.Context, in *message.TLMessageGetUserMessageList) (*message.Vector_MessageBox, error) {
	var datas []*mtproto.MessageBox
	for _, id := range in.GetIdList() {
		if box := f.byID[id]; box != nil {
			datas = append(datas, box)
		}
	}
	return &message.Vector_MessageBox{Datas: datas}, nil
}
