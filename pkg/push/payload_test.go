package push

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBuildAlertIOSNotificationServiceShape(t *testing.T) {
	secret := bytes.Repeat([]byte{0x5a}, 256)
	msg := Incoming{
		UserID:     9,
		MsgID:      42,
		SenderID:   3,
		SenderName: "Ada Lovelace",
		Text:       "hello",
		LocKey:     "MESSAGE_TEXT",
		PeerType:   mtproto.PEER_USER,
		PeerID:     3,
	}
	alert, err := BuildAlert(msg, Decision{Send: true, Preview: true, Sound: "default"}, secret)
	if err != nil {
		t.Fatal(err)
	}
	var outer map[string]any
	if err := json.Unmarshal(alert.Outer, &outer); err != nil {
		t.Fatal(err)
	}
	aps := outer["aps"].(map[string]any)
	if aps["mutable-content"].(float64) != 1 {
		t.Fatalf("mutable-content %#v", aps["mutable-content"])
	}
	alertObj := aps["alert"].(map[string]any)
	if alertObj["loc-key"] != "MESSAGE_TEXT" {
		t.Fatal(alertObj["loc-key"])
	}
	args := alertObj["loc-args"].([]any)
	if args[0] != "Ada Lovelace" || args[1] != "hello" {
		t.Fatal(args)
	}
	if outer["from_id"] != "3" || outer["msg_id"] != "42" {
		t.Fatalf("custom ids %#v %#v", outer["from_id"], outer["msg_id"])
	}
	custom := outer["custom"].(map[string]any)
	if custom["from_id"] != "3" || custom["msg_id"] != "42" {
		t.Fatal(custom)
	}
	p, _ := outer["p"].(string)
	plain, err := DecryptNotification(secret, p)
	if err != nil {
		t.Fatal(err)
	}
	var inner map[string]any
	if err := json.Unmarshal(plain, &inner); err != nil {
		t.Fatal(err)
	}
	if _, ok := inner["loc-key"]; ok {
		t.Fatal("decrypted payload must not include loc-key or NotificationService skips the banner")
	}
	if inner["from_id"] != "3" || inner["msg_id"] != "42" {
		t.Fatalf("inner ids %#v", inner)
	}
	innerAPS := inner["aps"].(map[string]any)
	innerAlert := innerAPS["alert"].(map[string]any)
	if innerAlert["title"] != "Ada Lovelace" || innerAlert["body"] != "hello" {
		t.Fatalf("inner alert %#v", innerAlert)
	}
}

func TestGroupPayloadUsesChatIDNotFromID(t *testing.T) {
	msg := Incoming{
		UserID: 9, MsgID: 8, SenderID: 3, SenderName: "Ada", ChatID: 77, ChatTitle: "Lab",
		Text: "hi", LocKey: "CHAT_MESSAGE_TEXT", PeerType: mtproto.PEER_CHAT, PeerID: 77,
	}
	alert, err := BuildAlert(msg, Decision{Send: true, Preview: true, Sound: "default"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var outer map[string]any
	if err := json.Unmarshal(alert.Outer, &outer); err != nil {
		t.Fatal(err)
	}
	if _, ok := outer["p"]; ok {
		t.Fatal("no secret should mean no p")
	}
	if outer["chat_id"] != "77" || outer["from_id"] != nil {
		t.Fatalf("%#v", outer)
	}
	if outer["aps"].(map[string]any)["alert"].(map[string]any)["loc-key"] != "CHAT_MESSAGE_TEXT" {
		t.Fatal(outer)
	}
	hidden, err := BuildAlert(msg, Decision{Send: true, Preview: false, Sound: "default"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var hid map[string]any
	if err := json.Unmarshal(hidden.Outer, &hid); err != nil {
		t.Fatal(err)
	}
	if hid["aps"].(map[string]any)["alert"].(map[string]any)["loc-key"] != "CHAT_MESSAGE_NOTEXT" {
		t.Fatal(hid)
	}
}

func TestMessagesFromUpdatesSkipsOutgoing(t *testing.T) {
	user := mtproto.MakeTLUser(&mtproto.User{
		Id:        3,
		FirstName: &wrapperspb.StringValue{Value: "Ada"},
	}).To_User()
	in := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     false,
		Id:      42,
		Message: "hello",
		FromId:  mtproto.MakePeerUser(3),
		PeerId:  mtproto.MakePeerUser(3),
	}).To_Message()
	outMsg := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		Id:      43,
		Message: "mine",
		FromId:  mtproto.MakePeerUser(9),
		PeerId:  mtproto.MakePeerUser(3),
	}).To_Message()
	updates := mtproto.MakeUpdatesByUpdatesUsers(nil,
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: in}).To_Update(),
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: outMsg}).To_Update(),
	)
	updates.Users = []*mtproto.User{user}
	got := Messages(9, updates)
	if len(got) != 1 || got[0].MsgID != 42 || got[0].SenderName != "Ada" || got[0].LocKey != "MESSAGE_TEXT" {
		t.Fatalf("%+v", got)
	}
}
