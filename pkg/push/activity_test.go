package push

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

type peerSettings struct {
	MemoryStore
	byPeer map[int64]NotifySettings
	global NotifySettings
}

func (p *peerSettings) NotifySettings(_ context.Context, _ int64, _ int32, peerID int64) (NotifySettings, error) {
	if peerID == 0 {
		return p.global, nil
	}
	if row, ok := p.byPeer[peerID]; ok {
		return row, nil
	}
	return NotifySettings{}, nil
}

func TestDeliverActivityPushesLikeInChinese(t *testing.T) {
	store := &peerSettings{global: NotifySettings{Found: true, MuteUntil: int32(time.Now().Unix()) + 3600}}
	ctx := context.Background()
	secret := bytesRepeat(256)
	_ = store.Upsert(ctx, Device{AuthKeyID: 1, UserID: 9, TokenType: TokenFCM, Token: "fcm", Secret: secret, LangCode: "zh-Hans"})
	_ = store.Upsert(ctx, Device{AuthKeyID: 2, UserID: 9, TokenType: TokenHuawei, Token: "hms", Secret: secret, LangCode: "zh-Hans"})
	_ = store.Upsert(ctx, Device{AuthKeyID: 3, UserID: 9, TokenType: TokenAPNSVoIP, Token: "voip", Secret: secret})
	fcm := &captureFCM{}
	hms := &captureHuawei{}
	svc := NewService(store, Config{})
	svc.SetFCM(fcm)
	svc.SetHuawei(hms)
	n, err := svc.DeliverActivity(ctx, Activity{UserID: 9, Kind: "like", ActorName: "林", PostID: "post-1"})
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var data map[string]string
	if err := json.Unmarshal(fcm.body, &data); err != nil {
		t.Fatal(err)
	}
	if data["loc_key"] != "DISCOVER_LIKE" || !strings.Contains(data["body"], "赞了你的帖子") {
		t.Fatalf("fcm %#v", data)
	}
	if !strings.Contains(data["custom"], `"discover":"1"`) || !strings.Contains(data["custom"], `"post_id":"post-1"`) {
		t.Fatalf("custom %s", data["custom"])
	}
	if hms.data == "" {
		t.Fatal("huawei payload empty")
	}
	plain, err := DecryptNotification(secret, hms.data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plain), `"loc_key":"DISCOVER_LIKE"`) || !strings.Contains(string(plain), `"post_id":"post-1"`) {
		t.Fatalf("huawei %s", plain)
	}
	alert, err := BuildAlert(Activity{UserID: 9, Kind: "like", ActorName: "林", PostID: "post-1"}.incoming(), Decision{Send: true, Preview: true, Sound: "default"}, nil, "zh-Hans")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(alert.Outer), `"loc-key":"DISCOVER_LIKE"`) || strings.Contains(string(alert.Inner), "loc-key") {
		t.Fatalf("apns outer %s inner %s", alert.Outer, alert.Inner)
	}
}

func TestDeliverActivityHidesCommentWhenPreviewOff(t *testing.T) {
	store := &peerSettings{byPeer: map[int64]NotifySettings{
		DiscoverNotifyUserID: {Found: true, ShowPreviews: 0},
	}}
	ctx := context.Background()
	_ = store.Upsert(ctx, Device{AuthKeyID: 1, UserID: 9, TokenType: TokenFCM, Token: "fcm", Secret: bytesRepeat(256), LangCode: "en"})
	fcm := &captureFCM{}
	svc := NewService(store, Config{})
	svc.SetFCM(fcm)
	if _, err := svc.DeliverActivity(ctx, Activity{UserID: 9, Kind: "comment", ActorName: "林", PostID: "p", Text: "secret words"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fcm.body), "secret") || !strings.Contains(string(fcm.body), "DISCOVER_COMMENT") {
		t.Fatalf("%s", fcm.body)
	}
	if strings.Contains(string(fcm.body), "DISCOVER_COMMENT_TEXT") {
		t.Fatalf("preview off kept the text key: %s", fcm.body)
	}
}

func TestDeliverActivityMuteIsOnlyTheDiscoverPeer(t *testing.T) {
	future := int32(time.Now().Unix()) + 3600
	store := &peerSettings{byPeer: map[int64]NotifySettings{
		DiscoverNotifyUserID: {Found: true, MuteUntil: future, ShowPreviews: 1},
	}}
	_ = store.Upsert(context.Background(), Device{AuthKeyID: 1, UserID: 9, TokenType: TokenFCM, Token: "fcm", Secret: bytesRepeat(256)})
	fcm := &captureFCM{}
	svc := NewService(store, Config{})
	svc.SetFCM(fcm)
	_, err := svc.DeliverActivity(context.Background(), Activity{UserID: 9, Kind: "follow", ActorName: "林"})
	if err != ErrMuted || fcm.token != "" {
		t.Fatalf("err=%v token=%q", err, fcm.token)
	}

	store.byPeer[DiscoverNotifyUserID] = NotifySettings{Found: true, ShowPreviews: 1}
	store.global = NotifySettings{Found: true, MuteUntil: future}
	if _, err := svc.DeliverActivity(context.Background(), Activity{UserID: 9, Kind: "follow", ActorName: "林"}); err != nil || fcm.token != "fcm" {
		t.Fatalf("global mute blocked discover: %v token %q", err, fcm.token)
	}
}

func TestDeliverActivitySendsXiaomi(t *testing.T) {
	store := &peerSettings{}
	ctx := context.Background()
	secret := bytesRepeat(256)
	_ = store.Upsert(ctx, Device{AuthKeyID: 4, UserID: 9, TokenType: TokenXiaomi, Token: "mi", Secret: secret, LangCode: "zh-Hans"})
	cap := &captureXiaomi{}
	svc := NewService(store, Config{})
	svc.SetXiaomi(cap)
	n, err := svc.DeliverActivity(ctx, Activity{UserID: 9, Kind: "repost", ActorName: "林", PostID: "post-2"})
	if err != nil || n != 1 || cap.token != "mi" || cap.msg.Payload == "" {
		t.Fatalf("n=%d err=%v token=%q", n, err, cap.token)
	}
	plain, err := DecryptNotification(secret, cap.msg.Payload)
	if err != nil || !strings.Contains(string(plain), "DISCOVER_REPOST") {
		t.Fatalf("%v %s", err, plain)
	}
}

func TestUnconfiguredXiaomiDoesNotCountAsSent(t *testing.T) {
	store := &peerSettings{}
	_ = store.Upsert(context.Background(), Device{AuthKeyID: 4, UserID: 9, TokenType: TokenXiaomi, Token: "mi", Secret: bytesRepeat(256)})
	svc := NewService(store, Config{})
	n, err := svc.DeliverActivity(context.Background(), Activity{UserID: 9, Kind: "like", PostID: "p"})
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestChatPushDoesNotGainDiscoverFields(t *testing.T) {
	alert, err := BuildAlert(Incoming{UserID: 9, MsgID: 4, SenderID: 3, SenderName: "Ada", Text: "ping", LocKey: "MESSAGE_TEXT", PeerType: mtproto.PEER_USER, PeerID: 3}, Decision{Send: true, Preview: true}, nil, "en")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(alert.Outer), "discover") {
		t.Fatalf("%s", alert.Outer)
	}
}
