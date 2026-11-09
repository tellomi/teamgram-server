package push

import (
	"bytes"
	"encoding/json"
	"strings"
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
	alert, err := BuildAlert(msg, Decision{Send: true, Preview: true, Sound: "default"}, secret, "")
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
	if alertObj["title"] != "Ada Lovelace" || alertObj["body"] != "hello" {
		t.Fatalf("outer alert %#v", alertObj)
	}
	if _, ok := alertObj["subtitle"]; ok {
		t.Fatal("private chat outer alert has a subtitle")
	}
}

func TestGroupPayloadUsesChatIDNotFromID(t *testing.T) {
	msg := Incoming{
		UserID: 9, MsgID: 8, SenderID: 3, SenderName: "Ada", ChatID: 77, ChatTitle: "Lab",
		Text: "hi", LocKey: "CHAT_MESSAGE_TEXT", PeerType: mtproto.PEER_CHAT, PeerID: 77,
	}
	alert, err := BuildAlert(msg, Decision{Send: true, Preview: true, Sound: "default"}, nil, "")
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
	hidden, err := BuildAlert(msg, Decision{Send: true, Preview: false, Sound: "default"}, nil, "zh-Hans")
	if err != nil {
		t.Fatal(err)
	}
	var hid map[string]any
	if err := json.Unmarshal(hidden.Outer, &hid); err != nil {
		t.Fatal(err)
	}
	hiddenAlert := hid["aps"].(map[string]any)["alert"].(map[string]any)
	if hiddenAlert["loc-key"] != "CHAT_MESSAGE_NOTEXT" {
		t.Fatal(hid)
	}
	if hiddenAlert["body"] != "新消息" {
		t.Fatalf("hidden body %#v", hiddenAlert["body"])
	}
	if _, ok := hiddenAlert["title"]; ok {
		t.Fatal("hidden preview added a title")
	}
	if _, ok := hiddenAlert["subtitle"]; ok {
		t.Fatal("hidden preview added a subtitle")
	}
	args := hiddenAlert["loc-args"].([]any)
	for _, a := range args {
		if a == "hi" {
			t.Fatal("hidden preview put the text in loc-args")
		}
	}
}

func TestOuterAlertShapes(t *testing.T) {
	secret := bytes.Repeat([]byte{0x5a}, 256)
	cases := []struct {
		name     string
		msg      Incoming
		preview  bool
		lang     string
		title    string
		subtitle string
		body     string
		locKey   string
		args     []any
	}{
		{
			name: "private",
			msg: Incoming{
				UserID: 9, MsgID: 42, SenderID: 3, SenderName: "Ada Lovelace",
				Text: "hello", LocKey: "MESSAGE_TEXT", PeerType: mtproto.PEER_USER, PeerID: 3,
			},
			preview: true,
			title:   "Ada Lovelace",
			body:    "hello",
			locKey:  "MESSAGE_TEXT",
			args:    []any{"Ada Lovelace", "hello"},
		},
		{
			name: "group",
			msg: Incoming{
				UserID: 9, MsgID: 8, SenderID: 3, SenderName: "Ada",
				ChatID: 77, ChatTitle: "Lab",
				Text: "hi", LocKey: "CHAT_MESSAGE_TEXT", PeerType: mtproto.PEER_CHAT, PeerID: 77,
			},
			preview:  true,
			title:    "Lab",
			subtitle: "Ada",
			body:     "hi",
			locKey:   "CHAT_MESSAGE_TEXT",
			args:     []any{"Ada", "Lab", "hi"},
		},
		{
			name: "channel",
			msg: Incoming{
				UserID: 9, MsgID: 11, SenderID: 3, SenderName: "Ada",
				ChannelID: 88, ChatTitle: "News",
				Text: "bulletin", LocKey: "CHANNEL_MESSAGE_TEXT", PeerType: mtproto.PEER_CHANNEL, PeerID: 88,
			},
			preview: true,
			title:   "News",
			body:    "bulletin",
			locKey:  "CHANNEL_MESSAGE_TEXT",
			args:    []any{"News", "bulletin"},
		},
		{
			name: "previews hidden",
			msg: Incoming{
				UserID: 9, MsgID: 8, SenderID: 3, SenderName: "Ada",
				ChatID: 77, ChatTitle: "Lab",
				Text: "secret words", LocKey: "CHAT_MESSAGE_TEXT", PeerType: mtproto.PEER_CHAT, PeerID: 77,
			},
			preview: false,
			lang:    "zh-Hans",
			body:    "新消息",
			locKey:  "CHAT_MESSAGE_NOTEXT",
			args:    []any{"Ada", "Lab"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alert, err := BuildAlert(tc.msg, Decision{Send: true, Preview: tc.preview, Sound: "default"}, secret, tc.lang)
			if err != nil {
				t.Fatal(err)
			}
			var outer map[string]any
			if err := json.Unmarshal(alert.Outer, &outer); err != nil {
				t.Fatal(err)
			}
			got := outer["aps"].(map[string]any)["alert"].(map[string]any)
			if got["loc-key"] != tc.locKey {
				t.Fatalf("loc-key %#v", got["loc-key"])
			}
			if title, _ := got["title"].(string); title != tc.title {
				t.Fatalf("title %#v", got["title"])
			}
			if subtitle, _ := got["subtitle"].(string); subtitle != tc.subtitle {
				t.Fatalf("subtitle %#v", got["subtitle"])
			}
			body, _ := got["body"].(string)
			if body != tc.body {
				t.Fatalf("body %#v", got["body"])
			}
			if tc.title != "" && hasNamePrefix(body, tc.subtitle, tc.title) {
				t.Fatalf("body still has the name prefix: %#v", body)
			}
			args := got["loc-args"].([]any)
			if len(args) != len(tc.args) {
				t.Fatal(args)
			}
			for i := range tc.args {
				if args[i] != tc.args[i] {
					t.Fatalf("loc-args %#v", args)
				}
			}
			if !tc.preview && (strings.Contains(body, tc.msg.Text) || strings.Contains(body, tc.msg.SenderName)) {
				t.Fatalf("hidden preview named the sender or the text: %#v", got)
			}
			plain, err := DecryptNotification(secret, outer["p"].(string))
			if err != nil {
				t.Fatal(err)
			}
			var inner map[string]any
			if err := json.Unmarshal(plain, &inner); err != nil {
				t.Fatal(err)
			}
			if _, ok := inner["loc-key"]; ok {
				t.Fatal("decrypted payload must not include loc-key")
			}
			innerAlert := inner["aps"].(map[string]any)["alert"].(map[string]any)
			// p is unchanged: the decrypted alert still carries the name even
			// when the outer alert hides it.
			wantTitle, wantSubtitle, _ := alertText(tc.msg, tc.msg.LocKeyFor(tc.preview), tc.lang, tc.preview)
			if innerAlert["title"] != wantTitle {
				t.Fatalf("inner title %#v", innerAlert["title"])
			}
			if innerAlert["subtitle"] != wantSubtitle && !(wantSubtitle == "" && innerAlert["subtitle"] == nil) {
				t.Fatalf("inner subtitle %#v", innerAlert["subtitle"])
			}
			if _, ok := innerAlert["loc-key"]; ok || innerAlert["title-loc-key"] != nil {
				t.Fatalf("inner alert %#v", innerAlert)
			}
		})
	}
}

func hasNamePrefix(body string, names ...string) bool {
	for _, name := range names {
		if name == "" {
			continue
		}
		if strings.HasPrefix(body, name+":") || strings.HasPrefix(body, name+"：") {
			return true
		}
	}
	return false
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
