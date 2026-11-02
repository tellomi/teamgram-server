package push

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestNormalizeLang(t *testing.T) {
	cases := map[string]string{
		"":        "en",
		"en":      "en",
		"en-US":   "en",
		"zh":      "zh-hans",
		"zh-CN":   "zh-hans",
		"zh_cn":   "zh-hans",
		"zh-Hans": "zh-hans",
		"zh-hans": "zh-hans",
		"zh-Hant": "zh-hant",
		"zh-TW":   "zh-hant",
		"zh-HK":   "zh-hant",
		"ja":      "en",
	}
	for in, want := range cases {
		if got := NormalizeLang(in); got != want {
			t.Fatalf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestChineseFallbackBody(t *testing.T) {
	secret := bytes.Repeat([]byte{0x5a}, 256)
	msg := Incoming{
		UserID: 9, MsgID: 42, SenderID: 3, SenderName: "林",
		LocKey: "MESSAGE_AUDIO", PeerType: mtproto.PEER_USER, PeerID: 3,
	}
	alert, err := BuildAlert(msg, Decision{Send: true, Preview: true, Sound: "default"}, secret, "zh-Hans")
	if err != nil {
		t.Fatal(err)
	}
	var outer map[string]any
	if err := json.Unmarshal(alert.Outer, &outer); err != nil {
		t.Fatal(err)
	}
	apsAlert := outer["aps"].(map[string]any)["alert"].(map[string]any)
	if apsAlert["loc-key"] != "MESSAGE_AUDIO" {
		t.Fatal(apsAlert["loc-key"])
	}
	args := apsAlert["loc-args"].([]any)
	if len(args) != 1 || args[0] != "林" {
		t.Fatal(args)
	}
	if apsAlert["body"] != "发来一条语音消息" {
		t.Fatalf("outer body %#v", apsAlert["body"])
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
		t.Fatal("iOS decrypted payload must not include loc-key")
	}
	innerBody := inner["aps"].(map[string]any)["alert"].(map[string]any)["body"]
	if innerBody != "发来一条语音消息" {
		t.Fatalf("inner body %#v", innerBody)
	}

	var data map[string]string
	if err := json.Unmarshal(alert.Android, &data); err != nil {
		t.Fatal(err)
	}
	if data["loc_key"] != "MESSAGE_AUDIO" || data["body"] != "发来一条语音消息" {
		t.Fatalf("%#v", data)
	}
	if data["p"] == "" || data["p"] == outer["p"].(string) {
		t.Fatal("android p must be present and must not be the iOS plaintext")
	}
	androidPlain, err := DecryptNotification(secret, data["p"])
	if err != nil {
		t.Fatal(err)
	}
	var opened map[string]any
	if err := json.Unmarshal(androidPlain, &opened); err != nil {
		t.Fatal(err)
	}
	if opened["loc_key"] != "MESSAGE_AUDIO" {
		t.Fatal(opened["loc_key"])
	}
	openedArgs := opened["loc_args"].([]any)
	if openedArgs[0] != "林" {
		t.Fatal(openedArgs)
	}
	if _, ok := opened["loc-key"]; ok {
		t.Fatal("android plaintext uses loc_key, not loc-key")
	}
	if alert.Huawei != data["p"] {
		t.Fatal("huawei payload must be the encrypted loc_key object")
	}

	photo := msg
	photo.LocKey = "MESSAGE_PHOTO"
	photoAlert, err := BuildAlert(photo, Decision{Send: true, Preview: true}, nil, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	var photoOuter map[string]any
	if err := json.Unmarshal(photoAlert.Outer, &photoOuter); err != nil {
		t.Fatal(err)
	}
	photoBody := photoOuter["aps"].(map[string]any)["alert"].(map[string]any)["body"]
	if photoBody != "发来一张图片" {
		t.Fatalf("photo body %#v", photoBody)
	}
	if photoAlert.Huawei != "" {
		t.Fatal("no secret should mean no huawei ciphertext")
	}
}

func TestLocKeysAndPhrases(t *testing.T) {
	kinds := []struct {
		k    kind
		peer int32
		key  string
		zh   string
		en   string
	}{
		{kindText, mtproto.PEER_USER, "MESSAGE_TEXT", "", ""},
		{kindNoText, mtproto.PEER_USER, "MESSAGE_NOTEXT", "发来一条消息", "sent you a message"},
		{kindPhoto, mtproto.PEER_USER, "MESSAGE_PHOTO", "发来一张图片", "sent you a photo"},
		{kindPhotoSecret, mtproto.PEER_USER, "MESSAGE_PHOTO_SECRET", "发来一张阅后即焚图片", "sent you a self-destructing photo"},
		{kindVideo, mtproto.PEER_USER, "MESSAGE_VIDEO", "发来一段视频", "sent you a video"},
		{kindVideoSecret, mtproto.PEER_USER, "MESSAGE_VIDEO_SECRET", "发来一段阅后即焚视频", "sent you a self-destructing video"},
		{kindGIF, mtproto.PEER_USER, "MESSAGE_GIF", "发来一张 GIF", "sent you a GIF"},
		{kindRound, mtproto.PEER_USER, "MESSAGE_ROUND", "发来一条视频消息", "sent you a video message"},
		{kindVoice, mtproto.PEER_USER, "MESSAGE_AUDIO", "发来一条语音消息", "sent you a voice message"},
		{kindSticker, mtproto.PEER_USER, "MESSAGE_STICKER", "发来一张贴纸", "sent you a sticker"},
		{kindDoc, mtproto.PEER_USER, "MESSAGE_DOC", "发来一个文件", "sent you a file"},
		{kindGeo, mtproto.PEER_USER, "MESSAGE_GEO", "发来一个位置", "sent you a map"},
		{kindGeoLive, mtproto.PEER_USER, "MESSAGE_GEOLIVE", "开始共享实时位置", "started sharing their live location"},
		{kindContact, mtproto.PEER_USER, "MESSAGE_CONTACT", "分享了一个联系人", "shared a contact"},
		{kindPoll, mtproto.PEER_USER, "MESSAGE_POLL", "发来一个投票", "sent you a poll"},
		{kindQuiz, mtproto.PEER_USER, "MESSAGE_QUIZ", "发来一个问答", "sent you a quiz"},
		{kindVoice, mtproto.PEER_CHAT, "CHAT_MESSAGE_AUDIO", "发来一条语音消息", "sent you a voice message"},
		{kindPhoto, mtproto.PEER_CHANNEL, "CHANNEL_MESSAGE_PHOTO", "发来一张图片", "sent you a photo"},
		{kindPin, mtproto.PEER_CHAT, "PINNED_NOTEXT", "置顶了一条消息", "pinned a message"},
		{kindPinText, mtproto.PEER_USER, "PINNED_TEXT", "置顶了一条消息", "pinned a message"},
	}
	for _, tc := range kinds {
		got := locKey(tc.peer, tc.k, "hello")
		if tc.k == kindText {
			got = locKey(tc.peer, tc.k, "hello")
		}
		if got != tc.key {
			t.Fatalf("kind %d peer %d: %q want %q", tc.k, tc.peer, got, tc.key)
		}
		if tc.zh == "" {
			continue
		}
		if phraseFor("zh-Hans", got) != tc.zh {
			t.Fatalf("%s zh %q", got, phraseFor("zh-Hans", got))
		}
		if phraseFor("en", got) != tc.en {
			t.Fatalf("%s en %q", got, phraseFor("en", got))
		}
		if phraseFor("zh-Hant", got) == tc.zh || phraseFor("zh-Hant", got) == "" {
			t.Fatalf("%s traditional should differ from simplified, got %q", got, phraseFor("zh-Hant", got))
		}
	}
	if phraseFor("zh-Hans", "REACT_AUDIO") != "回应了你的语音消息" {
		t.Fatal(phraseFor("zh-Hans", "REACT_AUDIO"))
	}
	if phraseFor("zh-Hans", "CHAT_REACT_PHOTO") != "回应了你的图片" {
		t.Fatal(phraseFor("zh-Hans", "CHAT_REACT_PHOTO"))
	}
}

func TestMessagesLocKeys(t *testing.T) {
	user := mtproto.MakeTLUser(&mtproto.User{
		Id:        3,
		FirstName: &wrapperspb.StringValue{Value: "林"},
	}).To_User()
	voice := mtproto.MakeTLMessage(&mtproto.Message{
		Out: false, Id: 7,
		FromId: mtproto.MakePeerUser(3),
		PeerId: mtproto.MakePeerUser(3),
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: mtproto.MakeTLDocument(&mtproto.Document{
				Id: 11,
				Attributes: []*mtproto.DocumentAttribute{
					mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: true}).To_DocumentAttribute(),
				},
			}).To_Document(),
		}).To_MessageMedia(),
	}).To_Message()
	photo := mtproto.MakeTLMessage(&mtproto.Message{
		Out: false, Id: 8,
		FromId: mtproto.MakePeerUser(3),
		PeerId: mtproto.MakePeerChat(77),
		Media:  mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia(),
	}).To_Message()
	secret := mtproto.MakeTLMessage(&mtproto.Message{
		Out: false, Id: 9,
		FromId: mtproto.MakePeerUser(3),
		PeerId: mtproto.MakePeerUser(3),
		Media: mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
			TtlSeconds: &wrapperspb.Int32Value{Value: 5},
		}).To_MessageMedia(),
	}).To_Message()
	pin := mtproto.MakeTLMessage(&mtproto.Message{
		Out:    false,
		Id:     10,
		FromId: mtproto.MakePeerUser(3),
		PeerId: mtproto.MakePeerChat(77),
		Action: mtproto.MakeTLMessageActionPinMessage(&mtproto.MessageAction{}).To_MessageAction(),
	}).To_Message()
	quiz := mtproto.MakeTLMessage(&mtproto.Message{
		Out: false, Id: 11,
		FromId: mtproto.MakePeerUser(3),
		PeerId: mtproto.MakePeerUser(3),
		Media: mtproto.MakeTLMessageMediaPoll(&mtproto.MessageMedia{
			Poll: mtproto.MakeTLPoll(&mtproto.Poll{Quiz: true}).To_Poll(),
		}).To_MessageMedia(),
	}).To_Message()
	updates := mtproto.MakeUpdatesByUpdatesUsers([]*mtproto.User{user},
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: voice}).To_Update(),
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: photo}).To_Update(),
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: secret}).To_Update(),
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: pin}).To_Update(),
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: quiz}).To_Update(),
	)
	updates.Chats = []*mtproto.Chat{mtproto.MakeTLChat(&mtproto.Chat{Id: 77, Title: "实验室"}).To_Chat()}
	got := Messages(9, updates)
	want := []string{"MESSAGE_AUDIO", "CHAT_MESSAGE_PHOTO", "MESSAGE_PHOTO_SECRET", "PINNED_NOTEXT", "MESSAGE_QUIZ"}
	if len(got) != len(want) {
		t.Fatalf("len %d %+v", len(got), got)
	}
	for i, key := range want {
		if got[i].LocKey != key {
			t.Fatalf("%d got %s want %s", i, got[i].LocKey, key)
		}
	}
	if got[1].ChatTitle != "实验室" {
		t.Fatal(got[1].ChatTitle)
	}
	alert, err := BuildAlert(got[0], Decision{Send: true, Preview: true, Sound: "default"}, nil, "zh-hans")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(alert.Outer), "发来一条语音消息") || !strings.Contains(string(alert.Outer), "MESSAGE_AUDIO") {
		t.Fatalf("%s", alert.Outer)
	}
	group, err := BuildAlert(got[1], Decision{Send: true, Preview: true}, nil, "zh-Hans")
	if err != nil {
		t.Fatal(err)
	}
	var g map[string]any
	if err := json.Unmarshal(group.Android, &g); err != nil {
		t.Fatal(err)
	}
	if g["loc_key"] != "CHAT_MESSAGE_PHOTO" || g["body"] != "发来一张图片" {
		t.Fatalf("%#v", g)
	}
	if !strings.Contains(g["loc_args"].(string), "林") || !strings.Contains(g["loc_args"].(string), "实验室") {
		t.Fatal(g["loc_args"])
	}
}

func TestDeliverUsesDeviceLang(t *testing.T) {
	store := &memSettings{peer: NotifySettings{Found: true, ShowPreviews: 1}}
	ctx := context.Background()
	secret := bytesRepeat(256)
	_ = store.Upsert(ctx, Device{
		AuthKeyID: 10, UserID: 9, TokenType: TokenFCM, Token: "fcm", Secret: secret, LangCode: "zh-Hans",
	})
	_ = store.Upsert(ctx, Device{
		AuthKeyID: 11, UserID: 9, TokenType: TokenHuawei, Token: "hms", Secret: secret, LangCode: "zh-Hans",
	})
	fcm := &captureFCM{}
	hms := &captureHuawei{}
	svc := NewService(store, Config{})
	svc.SetFCM(fcm)
	svc.SetHuawei(hms)
	svc.Deliver(ctx, 9, mediaUpdate(mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia()))
	if !strings.Contains(string(fcm.body), "发来一张图片") || !strings.Contains(string(fcm.body), "MESSAGE_PHOTO") {
		t.Fatalf("fcm %s", fcm.body)
	}
	var data map[string]string
	if err := json.Unmarshal(fcm.body, &data); err != nil {
		t.Fatal(err)
	}
	if hms.data == "" {
		t.Fatal("huawei should receive the encrypted loc_key payload")
	}
	plain, err := DecryptNotification(secret, hms.data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plain), `"loc_key":"MESSAGE_PHOTO"`) {
		t.Fatalf("huawei plain %s", plain)
	}
}

type captureHuawei struct {
	data string
}

func (c *captureHuawei) Send(_ context.Context, _ string, data string) error {
	c.data = data
	return nil
}

func mediaUpdate(media *mtproto.MessageMedia) *mtproto.Updates {
	user := mtproto.MakeTLUser(&mtproto.User{
		Id:        3,
		FirstName: &wrapperspb.StringValue{Value: "林"},
	}).To_User()
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Out: false, Id: 4, Media: media,
		FromId: mtproto.MakePeerUser(3), PeerId: mtproto.MakePeerUser(3),
	}).To_Message()
	return mtproto.MakeUpdatesByUpdatesUsers([]*mtproto.User{user},
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: msg}).To_Update())
}
