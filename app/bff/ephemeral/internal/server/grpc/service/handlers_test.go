package service

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/ephemeral/internal/box"
	"github.com/teamgram/teamgram-server/app/bff/ephemeral/internal/svc"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func withUser(userID int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: userID + 10}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func newSvc() *Service {
	return New(&svc.ServiceContext{Box: box.New()})
}

func mustEncode(t *testing.T, obj interface {
	Encode(*mtproto.EncodeBuf, int32) error
}) {
	t.Helper()
	buf := mtproto.NewEncodeBuf(2048)
	if err := obj.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
}

func TestEphemeral(t *testing.T) {
	svc := newSvc()
	ctx := withUser(9)
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8, AccessHash: 1}).To_InputPeer()
	receiver := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 8, AccessHash: 1}).To_InputUser()
	if _, err := svc.EphemeralSendMessage(context.Background(), &mtproto.TLEphemeralSendMessage{}); errText(err) != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
	ups, err := svc.EphemeralSendMessage(ctx, &mtproto.TLEphemeralSendMessage{
		Welcome:    true,
		Peer:       peer,
		ReceiverId: receiver,
		Message:    "welcome",
		RandomId:   55,
		ReplyTo:    mtproto.MakeTLInputReplyToEphemeralMessage(&mtproto.InputReplyTo{Id: 3}).To_InputReplyTo(),
		Anchor:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	mustEncode(t, ups)
	if ups.GetSeq() != 0 || len(ups.GetUpdates()) != 1 || ups.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateNewEphemeralMessage {
		t.Fatalf("send %#v", ups)
	}
	msg := ups.GetUpdates()[0].GetMessage_EPHEMERALMESSAGE()
	if msg.GetId() == 0 || !msg.GetWelcomeTemplate() || msg.GetAnchorMsgId().GetValue() != 3 || !msg.GetOut() {
		t.Fatalf("message %#v", msg)
	}
	again, err := svc.EphemeralSendMessage(ctx, &mtproto.TLEphemeralSendMessage{
		Peer: peer, ReceiverId: receiver, Message: "welcome", RandomId: 55,
	})
	if err != nil || again.GetUpdates()[0].GetMessage_EPHEMERALMESSAGE().GetId() != msg.GetId() {
		t.Fatal(err)
	}
	if _, err := svc.EphemeralSendMessage(ctx, &mtproto.TLEphemeralSendMessage{
		Peer: peer, ReceiverId: receiver, RandomId: 1,
	}); errText(err) != "MESSAGE_EMPTY" {
		t.Fatal(err)
	}
	if _, err := svc.EphemeralSendMessage(ctx, &mtproto.TLEphemeralSendMessage{
		Peer: peer, ReceiverId: receiver, Message: "x", RandomId: 2,
		Media: mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{}).To_InputMedia(),
	}); errText(err) != "MEDIA_INVALID" {
		t.Fatal(err)
	}

	edited, err := svc.EphemeralEditMessage(ctx, &mtproto.TLEphemeralEditMessage{
		Welcome:    true,
		Peer:       peer,
		ReceiverId: receiver,
		Id:         msg.GetId(),
		Message:    wrapperspb.String("welcome back"),
	})
	if err != nil {
		t.Fatal(err)
	}
	mustEncode(t, edited)
	if edited.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateEditEphemeralMessage {
		t.Fatal(edited.GetUpdates()[0].GetPredicateName())
	}
	if edited.GetUpdates()[0].GetMessage_EPHEMERALMESSAGE().GetMessage() != "welcome back" {
		t.Fatal(edited)
	}

	list, err := svc.EphemeralGetWelcomeMessages(ctx, &mtproto.TLEphemeralGetWelcomeMessages{Peer: peer})
	if err != nil || list.GetPredicateName() != mtproto.Predicate_ephemeral_welcomeMessages || len(list.GetMessages()) != 1 {
		t.Fatalf("list %#v %v", list, err)
	}
	mustEncode(t, list)
	same, err := svc.EphemeralGetWelcomeMessages(ctx, &mtproto.TLEphemeralGetWelcomeMessages{Peer: peer, Hash: list.GetHash()})
	if err != nil || same.GetPredicateName() != mtproto.Predicate_ephemeral_welcomeMessagesNotModified {
		t.Fatalf("not modified %#v %v", same, err)
	}
	mustEncode(t, same)

	ans, err := svc.EphemeralGetCallbackAnswer(ctx, &mtproto.TLEphemeralGetCallbackAnswer{Peer: peer, Id: msg.GetId(), Data: []byte("ok")})
	if err != nil || ans.GetAlert() || ans.GetMessage() != nil {
		t.Fatalf("callback %#v %v", ans, err)
	}
	mustEncode(t, ans)
	rep, err := svc.EphemeralReportMessage(ctx, &mtproto.TLEphemeralReportMessage{Peer: peer, Id: msg.GetId()})
	if err != nil || rep.GetPredicateName() != mtproto.Predicate_reportResultChooseOption || len(rep.GetOptions()) == 0 {
		t.Fatalf("report %#v %v", rep, err)
	}
	mustEncode(t, rep)
	done, err := svc.EphemeralReportMessage(ctx, &mtproto.TLEphemeralReportMessage{Peer: peer, Id: msg.GetId(), Option: []byte("spam")})
	if err != nil || done.GetPredicateName() != mtproto.Predicate_reportResultReported {
		t.Fatalf("reported %#v %v", done, err)
	}

	plain, err := svc.EphemeralSendMessage(ctx, &mtproto.TLEphemeralSendMessage{
		Peer: peer, ReceiverId: receiver, Message: "note", RandomId: 70,
	})
	if err != nil {
		t.Fatal(err)
	}
	plainID := plain.GetUpdates()[0].GetMessage_EPHEMERALMESSAGE().GetId()
	if _, err := svc.EphemeralDeleteWelcomeMessage(ctx, &mtproto.TLEphemeralDeleteWelcomeMessage{Peer: peer, Id: plainID}); errText(err) != "MESSAGE_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.EphemeralDeleteMessage(ctx, &mtproto.TLEphemeralDeleteMessage{Peer: peer, ReceiverId: receiver, Id: plainID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EphemeralDeleteAllWelcomeMessages(ctx, &mtproto.TLEphemeralDeleteAllWelcomeMessages{Peer: peer}); err != nil {
		t.Fatal(err)
	}
	after, err := svc.EphemeralGetWelcomeMessages(ctx, &mtproto.TLEphemeralGetWelcomeMessages{Peer: peer})
	if err != nil || len(after.GetMessages()) != 0 {
		t.Fatalf("cleared %#v %v", after, err)
	}
}
