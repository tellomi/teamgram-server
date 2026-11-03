package push

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/langpack"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestNormalizeLang(t *testing.T) {
	cases := map[string]string{
		"":              "en",
		"en":            "en",
		"en-US":         "en",
		"zh":            "zh-hans",
		"zh-CN":         "zh-hans",
		"zh_cn":         "zh-hans",
		"zh-Hans":       "zh-hans",
		"zh-hans":       "zh-hans",
		"zh-Hant":       "zh-hant",
		"zh-TW":         "zh-hant",
		"zh-HK":         "zh-hant",
		"classic-zh-cn": "classic-zh-cn",
		"taiwan":        "taiwan",
		"ja":            "ja",
		"pt_br":         "pt-br",
	}
	for in, want := range cases {
		if got := NormalizeLang(in); got != want {
			t.Fatalf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestPickLangPrefersAppThenPhone(t *testing.T) {
	if PickLang("zh-hans", "en") != "zh-hans" {
		t.Fatal(PickLang("zh-hans", "en"))
	}
	if PickLang("ja", "zh-CN") != "ja" {
		t.Fatal(PickLang("ja", "zh-CN"))
	}
	// The notification extension writes lang_code "en" and leaves the phone language.
	if PickLang("en", "zh-Hans-CN") != "zh-hans" {
		t.Fatal(PickLang("en", "zh-Hans-CN"))
	}
	if PickLang("", "zh-Hant-HK") != "zh-hant" {
		t.Fatal(PickLang("", "zh-Hant-HK"))
	}
	if PickLang("en", "en-US") != "en" {
		t.Fatal(PickLang("en", "en-US"))
	}
	if PickLang("classic-zh-cn", "en") != "classic-zh-cn" {
		t.Fatal(PickLang("classic-zh-cn", "en"))
	}
}

func TestPushPhrasesForEveryLanguage(t *testing.T) {
	cases := []struct {
		lang string
		want string
	}{
		{"zh-hans", "🎤 语音消息"},
		{"zh-hant", "🎤 語音訊息"},
		{"classic-zh-cn", "🎤 语音留言"},
		{"taiwan", "🎤 語音訊息"},
		{"en", "sent you a voice message"},
		{"ja", "🎤 音声"},
	}
	for _, tc := range cases {
		if got := phraseFor(tc.lang, "MESSAGE_AUDIO"); got != tc.want {
			t.Fatalf("%s: %q want %q", tc.lang, got, tc.want)
		}
	}
	// A Chinese phone whose saved app language was overwritten with "en".
	if got := phraseFor(PickLang("en", "zh-Hans"), "MESSAGE_AUDIO"); got != "🎤 语音消息" {
		t.Fatal(got)
	}
	if got := phraseFor(PickLang("en", "zh-Hans"), "MESSAGE_PHOTO"); strings.Contains(got, "sent you") {
		t.Fatal(got)
	}
	if got := phraseFor("ru", "MESSAGE_AUDIO"); !strings.Contains(got, "Голос") && !strings.Contains(got, "🎤") {
		t.Fatal(got)
	}

	cat := langpack.DefaultCatalog()
	langs, err := cat.Languages("ios")
	if err != nil {
		t.Fatal(err)
	}
	if len(langs) < 30 {
		t.Fatalf("languages %d", len(langs))
	}
	en := phraseFor("en", "MESSAGE_AUDIO")
	for _, lang := range langs {
		code := lang.GetLangCode()
		body := phraseFor(code, "MESSAGE_AUDIO")
		if body == "" {
			t.Fatalf("%s empty", code)
		}
		low := strings.ToLower(body)
		if strings.Contains(low, "encrypt") || strings.Contains(body, "加密") || strings.Contains(body, "暗号化") {
			t.Fatalf("%s mentions encryption: %q", code, body)
		}
		if _, ok := cat.Plain("ios", code, "PUSH_MESSAGE_AUDIO"); !ok {
			t.Fatalf("%s missing PUSH_MESSAGE_AUDIO even via base", code)
		}
		if code == "en" && body != en {
			t.Fatalf("en %q", body)
		}
	}
}

func TestMessageTextKeepsALeadingName(t *testing.T) {
	msg := Incoming{SenderName: "林", Text: "林你好", LocKey: "MESSAGE_TEXT", PeerType: mtproto.PEER_USER, PeerID: 3}
	_, _, body := alertText(msg, "MESSAGE_TEXT", "zh-hans", true)
	if body != "林你好" {
		t.Fatal(body)
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
	if apsAlert["body"] != "🎤 语音消息" {
		t.Fatalf("outer body %#v", apsAlert["body"])
	}
	if apsAlert["title"] != nil {
		t.Fatal("outer alert has no title; the name is a loc-arg")
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
	innerAlert := inner["aps"].(map[string]any)["alert"].(map[string]any)
	if innerAlert["title"] != "林" || strings.Contains(innerAlert["title"].(string), "Tellomi") || strings.Contains(innerAlert["title"].(string), "→") {
		t.Fatalf("private title is the sender alone, got %#v", innerAlert["title"])
	}
	if innerAlert["body"] != "🎤 语音消息" {
		t.Fatalf("inner body %#v", innerAlert["body"])
	}

	var data map[string]string
	if err := json.Unmarshal(alert.Android, &data); err != nil {
		t.Fatal(err)
	}
	if data["loc_key"] != "MESSAGE_AUDIO" || data["body"] != "🎤 语音消息" {
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
	if photoBody != "📷图片" {
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
	}{
		{kindText, mtproto.PEER_USER, "MESSAGE_TEXT"},
		{kindNoText, mtproto.PEER_USER, "MESSAGE_NOTEXT"},
		{kindPhoto, mtproto.PEER_USER, "MESSAGE_PHOTO"},
		{kindPhotoSecret, mtproto.PEER_USER, "MESSAGE_PHOTO_SECRET"},
		{kindVideo, mtproto.PEER_USER, "MESSAGE_VIDEO"},
		{kindVideoSecret, mtproto.PEER_USER, "MESSAGE_VIDEO_SECRET"},
		{kindGIF, mtproto.PEER_USER, "MESSAGE_GIF"},
		{kindRound, mtproto.PEER_USER, "MESSAGE_ROUND"},
		{kindVoice, mtproto.PEER_USER, "MESSAGE_AUDIO"},
		{kindSticker, mtproto.PEER_USER, "MESSAGE_STICKER"},
		{kindDoc, mtproto.PEER_USER, "MESSAGE_DOC"},
		{kindGeo, mtproto.PEER_USER, "MESSAGE_GEO"},
		{kindGeoLive, mtproto.PEER_USER, "MESSAGE_GEOLIVE"},
		{kindContact, mtproto.PEER_USER, "MESSAGE_CONTACT"},
		{kindPoll, mtproto.PEER_USER, "MESSAGE_POLL"},
		{kindQuiz, mtproto.PEER_USER, "MESSAGE_QUIZ"},
		{kindVoice, mtproto.PEER_CHAT, "CHAT_MESSAGE_AUDIO"},
		{kindPhoto, mtproto.PEER_CHANNEL, "CHANNEL_MESSAGE_PHOTO"},
		{kindPin, mtproto.PEER_CHAT, "PINNED_NOTEXT"},
		{kindPinText, mtproto.PEER_USER, "PINNED_TEXT"},
	}
	for _, tc := range kinds {
		got := locKey(tc.peer, tc.k, "hello")
		if got != tc.key {
			t.Fatalf("kind %d peer %d: %q want %q", tc.k, tc.peer, got, tc.key)
		}
		if tc.k == kindText {
			continue
		}
		zh := phraseFor("zh-Hans", got)
		en := phraseFor("en", got)
		hant := phraseFor("zh-Hant", got)
		if zh == "" || en == "" || hant == "" {
			t.Fatalf("%s empty zh=%q en=%q hant=%q", got, zh, en, hant)
		}
		if zh == en {
			t.Fatalf("%s simplified matches English %q", got, zh)
		}
		if strings.Contains(strings.ToLower(zh), "encrypt") || strings.Contains(zh, "加密") {
			t.Fatalf("%s mentions encryption: %q", got, zh)
		}
	}
	if phraseFor("en", "MESSAGE_AUDIO") != "sent you a voice message" {
		t.Fatal(phraseFor("en", "MESSAGE_AUDIO"))
	}
	if !strings.Contains(phraseFor("zh-Hans", "REACT_AUDIO"), "语音") {
		t.Fatal(phraseFor("zh-Hans", "REACT_AUDIO"))
	}
	if !strings.Contains(phraseFor("zh-Hans", "CHAT_REACT_PHOTO"), "图") && !strings.Contains(phraseFor("zh-Hans", "CHAT_REACT_PHOTO"), "照") {
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
	if !strings.Contains(string(alert.Outer), "语音消息") || !strings.Contains(string(alert.Outer), "MESSAGE_AUDIO") {
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
	if g["loc_key"] != "CHAT_MESSAGE_PHOTO" || !strings.Contains(g["body"].(string), "图") {
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
		AuthKeyID: 11, UserID: 9, TokenType: TokenHuawei, Token: "hms", Secret: secret, LangCode: "en", SystemLangCode: "zh-Hans-CN",
	})
	fcm := &captureFCM{}
	hms := &captureHuawei{}
	svc := NewService(store, Config{})
	svc.SetFCM(fcm)
	svc.SetHuawei(hms)
	svc.Deliver(ctx, 9, mediaUpdate(mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia()))
	if !strings.Contains(string(fcm.body), "图") || !strings.Contains(string(fcm.body), "MESSAGE_PHOTO") {
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

func TestLoginCodeSpoilerHiddenInPushPreview(t *testing.T) {
	const code = "123456"
	text, ents := mtproto.MakeTextAndMessageEntities([]mtproto.MessageBuildEntry{
		{Param: "登录验证码：", EntityType: mtproto.Predicate_messageEntityBold},
		{Param: code, EntityType: mtproto.Predicate_messageEntitySpoiler},
		{Text: "。请不要把此验证码告诉任何人。"},
	})
	user := mtproto.MakeTLUser(&mtproto.User{
		Id:        777000,
		FirstName: &wrapperspb.StringValue{Value: "Tellomi"},
	}).To_User()
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Out: false, Id: 15, Message: text, Entities: ents,
		FromId: mtproto.MakePeerUser(777000),
		PeerId: mtproto.MakePeerUser(9),
	}).To_Message()
	got := Messages(9, mtproto.MakeUpdatesByUpdatesUsers([]*mtproto.User{user},
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: msg}).To_Update(),
	))
	if len(got) != 1 {
		t.Fatalf("incoming %+v", got)
	}
	if strings.Contains(got[0].Text, code) {
		t.Fatalf("push preview leaked the code: %q", got[0].Text)
	}
	if !strings.Contains(got[0].Text, "登录验证码：") || !strings.Contains(got[0].Text, "••••••") {
		t.Fatalf("preview %q", got[0].Text)
	}
	alert, err := BuildAlert(got[0], Decision{Send: true, Preview: true, Sound: "default"}, nil, "zh-hans")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(alert.Outer), code) || strings.Contains(string(alert.Inner), code) || strings.Contains(string(alert.Android), code) {
		t.Fatalf("banner leaked the code outer=%s inner=%s android=%s", alert.Outer, alert.Inner, alert.Android)
	}
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
