package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestReplyTargetIDSurvivesMessageJSON(t *testing.T) {
	in := mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId: 41,
		TopMsgId:     wrapperspb.Int32(41),
	}).To_InputReplyTo()
	header := replyHeaderFromInput(7, mtproto.MakePeerUtil(mtproto.PEER_USER, 8), in)
	raw, loaded := roundTripPrivateMessage(t, header)
	h := loaded.GetReplyTo()
	// Private history stores the message as JSON. The send path writes both
	// reply_to_msg_id and reply_to_msg_id_FLAGINT32.
	if !strings.Contains(raw, `"reply_to_msg_id":41`) || !strings.Contains(raw, `"reply_to_msg_id_FLAGINT32":`) {
		t.Fatalf("send header json %s", raw)
	}
	if h.GetReplyToMsgId() != 41 || h.GetReplyToMsgId_FLAGINT32().GetValue() != 41 {
		t.Fatalf("stored send header plain=%d flag=%d int32=%d", h.GetReplyToMsgId(), h.GetReplyToMsgId_FLAGINT32().GetValue(), h.GetReplyToMsgId_INT32())
	}
	if id := replyTargetID(loaded.FixData()); id != 41 {
		t.Fatalf("after json id=%d header=%+v", id, h)
	}

	plain := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{ReplyToMsgId: 41}).To_MessageReplyHeader()
	raw, loaded = roundTripPrivateMessage(t, plain)
	h = loaded.GetReplyTo()
	if !strings.Contains(raw, `"reply_to_msg_id":41`) || strings.Contains(raw, `"reply_to_msg_id_FLAGINT32"`) {
		t.Fatalf("plain-only json %s", raw)
	}
	if h.GetReplyToMsgId() != 41 || h.GetReplyToMsgId_FLAGINT32() != nil {
		t.Fatalf("plain-only loaded plain=%d flag=%v", h.GetReplyToMsgId(), h.GetReplyToMsgId_FLAGINT32())
	}
	if replyTargetID(loaded) != 41 || replyTargetID(loaded.FixData()) != 41 {
		t.Fatal("plain-only did not resolve")
	}

	flag := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId_FLAGINT32: wrapperspb.Int32(41),
	}).To_MessageReplyHeader()
	raw, loaded = roundTripPrivateMessage(t, flag)
	h = loaded.GetReplyTo()
	if !strings.Contains(raw, `"reply_to_msg_id_FLAGINT32":`) || strings.Contains(raw, `"reply_to_msg_id":`) {
		t.Fatalf("flag-only json %s", raw)
	}
	if h.GetReplyToMsgId() != 0 || h.GetReplyToMsgId_FLAGINT32().GetValue() != 41 {
		t.Fatalf("flag-only loaded plain=%d flag=%d", h.GetReplyToMsgId(), h.GetReplyToMsgId_FLAGINT32().GetValue())
	}
	if replyTargetID(loaded) != 41 || replyTargetID(loaded.FixData()) != 41 {
		t.Fatal("flag-only did not resolve")
	}

	withTopic := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId: 50,
		ReplyToTopId: wrapperspb.Int32(10),
	}).To_MessageReplyHeader()
	if id := replyTargetID(&mtproto.Message{ReplyTo: withTopic}); id != 50 {
		t.Fatalf("reply id %d", id)
	}
	topicOnly := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToTopId: wrapperspb.Int32(10),
	}).To_MessageReplyHeader()
	if id := replyTargetID(&mtproto.Message{ReplyTo: topicOnly}); id != 0 {
		t.Fatalf("topic root %d", id)
	}

	for _, hdr := range []*mtproto.MessageReplyHeader{plain, flag} {
		raw, err := proto.Marshal(&mtproto.Message{Message: "reply", ReplyTo: hdr})
		if err != nil {
			t.Fatal(err)
		}
		var back mtproto.Message
		if err = proto.Unmarshal(raw, &back); err != nil {
			t.Fatal(err)
		}
		if replyTargetID(&back) != 41 {
			t.Fatalf("proto header %+v", back.GetReplyTo())
		}
	}
}

func roundTripPrivateMessage(t *testing.T, header *mtproto.MessageReplyHeader) (string, *mtproto.Message) {
	t.Helper()
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      42,
		Message: "reply",
		PeerId:  mtproto.MakePeerUser(8),
		FromId:  mtproto.MakePeerUser(7),
		ReplyTo: header,
	}).To_Message()
	raw, err := jsonx.MarshalToString(msg)
	if err != nil {
		t.Fatal(err)
	}
	var loaded *mtproto.Message
	if err = jsonx.UnmarshalFromString(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	return raw, loaded
}

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

func TestSendReplyTLKeepsReplyToMsgId(t *testing.T) {
	req := &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8}).To_InputPeer(),
		Message:     "reply-row-205",
		RandomId:    99,
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: 41,
		}).To_InputReplyTo(),
	}
	buf := mtproto.NewEncodeBuf(512)
	if err := req.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if len(raw) < 4 {
		t.Fatal("short encode")
	}
	decoded := &mtproto.TLMessagesSendMessage{Constructor: mtproto.CRC32_messages_sendMessage_fef48f62}
	if err := decoded.Decode(mtproto.NewDecodeBuf(raw[4:])); err != nil {
		t.Fatal(err)
	}
	if decoded.GetReplyTo().GetReplyToMsgId() != 41 {
		t.Fatalf("decoded reply %#v", decoded.GetReplyTo())
	}
	header := replyHeaderFromInput(7, mtproto.MakePeerUtil(mtproto.PEER_USER, 8), decoded.GetReplyTo())
	if header.GetReplyToMsgId() != 41 || header.GetReplyToMsgId_FLAGINT32().GetValue() != 41 {
		t.Fatalf("header %#v", header)
	}
}

func TestOutboxKafkaRoundTripKeepsReplyToMsgId(t *testing.T) {
	in := mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId: 41,
	}).To_InputReplyTo()
	header := replyHeaderFromInput(7, mtproto.MakePeerUtil(mtproto.PEER_USER, 8), in)
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		Id:      42,
		Message: "reply-row-205",
		PeerId:  mtproto.MakePeerUser(8),
		FromId:  mtproto.MakePeerUser(7),
		ReplyTo: header,
	}).To_Message()
	req := &inbox.TLInboxSendUserMessageToInboxV2{
		UserId:   7,
		Out:      true,
		FromId:   7,
		PeerType: mtproto.PEER_USER,
		PeerId:   8,
		BoxList: []*mtproto.MessageBox{{
			UserId: 7, MessageId: 42, SenderUserId: 7,
			PeerType: mtproto.PEER_USER, PeerId: 8, Message: msg,
		}},
	}
	raw, err := jsonx.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var back inbox.TLInboxSendUserMessageToInboxV2
	if err = json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("kafka json: %v", err)
	}
	if len(back.GetBoxList()) != 1 || back.BoxList[0].GetMessage() == nil {
		t.Fatalf("box lost: %s", raw)
	}
	stored, err := jsonx.Marshal(back.BoxList[0].GetMessage())
	if err != nil {
		t.Fatal(err)
	}
	var loaded *mtproto.Message
	if err = jsonx.Unmarshal(stored, &loaded); err != nil {
		t.Fatal(err)
	}
	loaded = loaded.FixData()
	h := loaded.GetReplyTo()
	t.Logf("after kafka plain=%d flag=%d int32=%d top=%d", h.GetReplyToMsgId(), h.GetReplyToMsgId_FLAGINT32().GetValue(), h.GetReplyToMsgId_INT32(), h.GetReplyToTopId().GetValue())
	if h.GetReplyToMsgId() != 41 {
		t.Fatalf("reply_to_msg_id dropped: %s", stored)
	}
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
