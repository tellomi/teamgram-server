package push

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBuildCallMatchesPhoneCallRequest(t *testing.T) {
	secret := bytes.Repeat([]byte{0x5a}, 256)
	updates := incomingCallUpdates(t)
	body, err := BuildCall(Call{
		UserID: 9, CallID: 100, AccessHash: 200, CallerID: 3, CallerName: "Ada Lovelace", Updates: updates,
	}, secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > voipPayloadLimit {
		t.Fatalf("payload %d exceeds voip limit", len(body))
	}
	var outer map[string]any
	if err := json.Unmarshal(body, &outer); err != nil {
		t.Fatal(err)
	}
	if outer["loc-key"] != locKeyPhoneCallRequest || outer["call_id"] != "100" || outer["call_ah"] != "200" || outer["from_id"] != "3" {
		t.Fatalf("%v", outer)
	}
	aps := outer["aps"].(map[string]any)
	alert := aps["alert"].(map[string]any)
	if alert["loc-key"] != locKeyPhoneCallRequest {
		t.Fatal(alert)
	}
	args := alert["loc-args"].([]any)
	if len(args) != 1 || args[0] != "Ada Lovelace" {
		t.Fatal(args)
	}
	if _, ok := outer["updates"]; ok {
		t.Fatal("outer envelope must not repeat the updates blob")
	}
	if _, ok := outer["custom"]; ok {
		t.Fatal("outer envelope must not repeat updates inside custom")
	}

	plain, err := DecryptNotification(secret, outer["p"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var inner map[string]any
	if err := json.Unmarshal(plain, &inner); err != nil {
		t.Fatal(err)
	}
	if inner["loc-key"] != locKeyPhoneCallRequest || inner["call_id"] != "100" || inner["call_ah"] != "200" || inner["from_id"] != "3" {
		t.Fatalf("%v", inner)
	}
	if _, ok := inner["custom"]; ok {
		t.Fatal("sealed p must not repeat updates inside custom")
	}
	assertCallUpdates(t, inner["updates"].(string))
	if _, err := BuildCall(Call{CallID: 100, AccessHash: 200, CallerID: 3, Updates: updates}, nil); err == nil {
		t.Fatal("missing secret must not produce a voip payload")
	}
}

func TestDeliverIncomingCallPushesVoIPTokenOnly(t *testing.T) {
	client, srv, got := newAPNSCapture(t, "app.tellomi.chat")
	defer srv.Close()
	store := &MemoryStore{}
	ctx := context.Background()
	secret := bytes.Repeat([]byte{0x11}, 256)
	_ = store.Upsert(ctx, Device{AuthKeyID: 1, UserID: 9, TokenType: TokenAPNS, Token: "alert-token", Secret: secret})
	_ = store.Upsert(ctx, Device{AuthKeyID: 2, UserID: 9, TokenType: TokenAPNSVoIP, Token: "voip-token", Secret: secret, AppSandbox: false})
	_ = store.Upsert(ctx, Device{AuthKeyID: 3, UserID: 9, TokenType: TokenAPNSVoIP, Token: "voip-nosecret"})
	svc := NewService(store, Config{})
	svc.apns = client

	svc.Deliver(ctx, 9, incomingCallUpdates(t))
	if got.path != "/3/device/voip-token" {
		t.Fatalf("path %s", got.path)
	}
	if got.header.Get("apns-topic") != "app.tellomi.chat.voip" || got.header.Get("apns-push-type") != "voip" || got.header.Get("apns-expiration") != "0" {
		t.Fatalf("%v", got.header)
	}
	var outer map[string]any
	if err := json.Unmarshal([]byte(got.body), &outer); err != nil {
		t.Fatal(err)
	}
	if outer["loc-key"] != locKeyPhoneCallRequest || outer["p"] == "" {
		t.Fatalf("%v", outer)
	}

	got.path = ""
	svc.Deliver(ctx, 9, textUpdate())
	if got.path != "/3/device/alert-token" || got.header.Get("apns-push-type") != "alert" || got.header.Get("apns-topic") != "app.tellomi.chat" {
		t.Fatalf("message path %s topic %s type %s", got.path, got.header.Get("apns-topic"), got.header.Get("apns-push-type"))
	}

	got.path = ""
	waiting := mtproto.MakeUpdatesByUpdatesUsers(nil,
		mtproto.MakeTLUpdatePhoneCall(&mtproto.Update{
			PhoneCall: mtproto.MakeTLPhoneCallWaiting(&mtproto.PhoneCall{Id: 1, AccessHash: 2, AdminId: 3, ParticipantId: 9}).To_PhoneCall(),
		}).To_Update())
	svc.Deliver(ctx, 9, waiting)
	if got.path != "" {
		t.Fatal("phoneCallWaiting was pushed")
	}
}

func TestIncomingCallWithoutCallerIsNotSent(t *testing.T) {
	client, srv, got := newAPNSCapture(t, "app.tellomi.chat")
	defer srv.Close()
	store := &MemoryStore{}
	ctx := context.Background()
	_ = store.Upsert(ctx, Device{AuthKeyID: 2, UserID: 9, TokenType: TokenAPNSVoIP, Token: "voip-token", Secret: bytes.Repeat([]byte{0x22}, 256)})
	svc := NewService(store, Config{})
	svc.apns = client
	bare := mtproto.MakeUpdatesByUpdatesUsers(nil, mtproto.MakeTLUpdatePhoneCall(&mtproto.Update{
		PhoneCall: requestedCall(),
	}).To_Update())
	if ShouldDeliver(9, bare) != true {
		t.Fatal("handler should still see the request")
	}
	svc.Deliver(ctx, 9, bare)
	if got.path != "" {
		t.Fatal("unsafe voip push was sent")
	}
}

func TestIncomingCallReachesFCMAndHuawei(t *testing.T) {
	client, srv, got := newAPNSCapture(t, "app.tellomi.chat")
	defer srv.Close()
	store := &MemoryStore{}
	ctx := context.Background()
	secret := bytes.Repeat([]byte{0x11}, 256)
	_ = store.Upsert(ctx, Device{AuthKeyID: 1, UserID: 9, TokenType: TokenAPNS, Token: "alert-token", Secret: secret})
	_ = store.Upsert(ctx, Device{AuthKeyID: 2, UserID: 9, TokenType: TokenAPNSVoIP, Token: "voip-token", Secret: secret})
	_ = store.Upsert(ctx, Device{AuthKeyID: 3, UserID: 9, TokenType: TokenFCM, Token: "fcm-token", Secret: secret})
	_ = store.Upsert(ctx, Device{AuthKeyID: 4, UserID: 9, TokenType: TokenHuawei, Token: "hms-token", Secret: secret})
	_ = store.Upsert(ctx, Device{AuthKeyID: 5, UserID: 9, TokenType: TokenFCM, Token: "fcm-nosecret"})
	fcm := &captureFCM{}
	hms := &captureHuawei{}
	svc := NewService(store, Config{})
	svc.apns = client
	svc.SetFCM(fcm)
	svc.SetHuawei(hms)

	svc.Deliver(ctx, 9, incomingCallUpdates(t))
	if got.path != "/3/device/voip-token" {
		t.Fatalf("apns path %s", got.path)
	}
	if got.header.Get("apns-push-type") != "voip" {
		t.Fatalf("apns type %s", got.header.Get("apns-push-type"))
	}
	if fcm.token != "fcm-token" {
		t.Fatalf("fcm token %q", fcm.token)
	}
	var data map[string]string
	if err := json.Unmarshal(fcm.body, &data); err != nil {
		t.Fatal(err)
	}
	if data["loc_key"] != locKeyPhoneCallRequest || data["call_id"] != "100" || data["call_ah"] != "200" || data["p"] == "" {
		t.Fatalf("%v", data)
	}
	plain, err := DecryptNotification(secret, data["p"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plain), `"loc_key":"`+locKeyPhoneCallRequest+`"`) {
		t.Fatalf("%s", plain)
	}
	assertCallUpdates(t, data["updates"])
	if hms.token != "hms-token" || hms.data == "" {
		t.Fatalf("huawei token %q", hms.token)
	}
	hplain, err := DecryptNotification(secret, hms.data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hplain), `"loc_key":"`+locKeyPhoneCallRequest+`"`) {
		t.Fatalf("%s", hplain)
	}
	if _, _, err := BuildAndroidCall(Call{CallID: 100, CallerID: 3, Updates: incomingCallUpdates(t)}, nil); err == nil {
		t.Fatal("missing secret must not produce an android call payload")
	}
}

func TestIncomingCallReachesFCMWhenAPNSIsOff(t *testing.T) {
	store := &MemoryStore{}
	secret := bytes.Repeat([]byte{0x11}, 256)
	_ = store.Upsert(context.Background(), Device{AuthKeyID: 3, UserID: 9, TokenType: TokenFCM, Token: "fcm-token", Secret: secret})
	fcm := &captureFCM{}
	svc := NewService(store, Config{})
	svc.SetFCM(fcm)
	svc.Deliver(context.Background(), 9, incomingCallUpdates(t))
	if fcm.token != "fcm-token" || !strings.Contains(string(fcm.body), locKeyPhoneCallRequest) {
		t.Fatalf("token %q body %s", fcm.token, fcm.body)
	}
}

func incomingCallUpdates(t *testing.T) *mtproto.Updates {
	t.Helper()
	user := mtproto.MakeTLUser(&mtproto.User{
		Id:         3,
		AccessHash: wrapperspb.Int64(99),
		FirstName:  wrapperspb.String("Ada"),
		LastName:   wrapperspb.String("Lovelace"),
	}).To_User()
	return mtproto.MakeUpdatesByUpdatesUsers([]*mtproto.User{user},
		mtproto.MakeTLUpdatePhoneCall(&mtproto.Update{PhoneCall: requestedCall()}).To_Update())
}

func requestedCall() *mtproto.PhoneCall {
	return mtproto.MakeTLPhoneCallRequested(&mtproto.PhoneCall{
		Id: 100, AccessHash: 200, Date: 1_700_000_000, AdminId: 3, ParticipantId: 9,
		GAHash: bytes.Repeat([]byte{0xab}, 32),
		Protocol: mtproto.MakeTLPhoneCallProtocol(&mtproto.PhoneCallProtocol{
			UdpP2P: true, UdpReflector: true, MinLayer: 65, MaxLayer: 92,
			LibraryVersions: []string{"5.0.0"},
		}).To_PhoneCallProtocol(),
	}).To_PhoneCall()
}

func assertCallUpdates(t *testing.T, encoded string) {
	t.Helper()
	got := mustDecodeUpdates(t, encoded)
	if len(got.GetUpdates()) != 1 || got.GetUpdates()[0].GetPhoneCall().GetPredicateName() != mtproto.Predicate_phoneCallRequested {
		t.Fatalf("%+v", got.GetUpdates())
	}
	call := got.GetUpdates()[0].GetPhoneCall()
	if call.GetId() != 100 || call.GetAccessHash() != 200 || call.GetAdminId() != 3 {
		t.Fatalf("%+v", call)
	}
	if callerUser(got, 3) == nil || callerUser(got, 3).GetFirstName().GetValue() != "Ada" {
		t.Fatalf("users %+v", got.GetUsers())
	}
}

func fatIncomingCallUpdates(t *testing.T) *mtproto.Updates {
	t.Helper()
	caller := mtproto.MakeTLUser(&mtproto.User{
		Id:         3,
		AccessHash: wrapperspb.Int64(99),
		FirstName:  wrapperspb.String("Ada"),
		LastName:   wrapperspb.String("Lovelace"),
		Photo: mtproto.MakeTLUserProfilePhoto(&mtproto.UserProfilePhoto{
			PhotoId: 99_000_000_000,
			DcId:    2,
		}).To_UserProfilePhoto(),
	}).To_User()
	users := []*mtproto.User{caller}
	// Production requestCall also attaches the callee and other bulky
	// user objects. Repeating those copies in the envelope and inside p
	// is what pushed the official-looking JSON over 5120 bytes.
	for i := 0; i < 6; i++ {
		users = append(users, mtproto.MakeTLUser(&mtproto.User{
			Id:        int64(20 + i),
			FirstName: wrapperspb.String(strings.Repeat("X", 400)),
			LastName:  wrapperspb.String(strings.Repeat("Y", 400)),
		}).To_User())
	}
	return mtproto.MakeUpdatesByUpdatesUsers(users,
		mtproto.MakeTLUpdatePhoneCall(&mtproto.Update{PhoneCall: requestedCall()}).To_Update())
}

func TestBuildCallStaysUnderVoIPLimitWithFatUsers(t *testing.T) {
	secret := bytes.Repeat([]byte{0x5a}, 256)
	fat := fatIncomingCallUpdates(t)
	plainBuf := mtproto.NewEncodeBuf(8192)
	if err := fat.Encode(plainBuf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	plainB64 := base64.RawURLEncoding.EncodeToString(plainBuf.GetBuf())
	// The previous envelope repeated updates four times (outer, custom, and
	// the same pair inside p). That combination exceeded Apple's limit.
	if 4*len(plainB64) < voipPayloadLimit {
		t.Fatalf("fixture not bulky enough: updates b64 %d, 4 copies %d", len(plainB64), 4*len(plainB64))
	}

	body, err := BuildCall(Call{
		UserID: 9, CallID: 100, AccessHash: 200, CallerID: 3, CallerName: "Ada Lovelace", Updates: fat,
	}, secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > voipPayloadLimit {
		t.Fatalf("payload %d exceeds voip limit %d", len(body), voipPayloadLimit)
	}
	var outer map[string]any
	if err := json.Unmarshal(body, &outer); err != nil {
		t.Fatal(err)
	}
	if _, ok := outer["updates"]; ok {
		t.Fatal("outer envelope must not repeat the updates blob")
	}
	plain, err := DecryptNotification(secret, outer["p"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var inner map[string]any
	if err := json.Unmarshal(plain, &inner); err != nil {
		t.Fatal(err)
	}
	assertCallUpdates(t, inner["updates"].(string))
	decoded := mustDecodeUpdates(t, inner["updates"].(string))
	if got := callerUser(decoded, 3); got == nil || got.GetFirstName().GetValue() != "Ada" {
		t.Fatalf("caller %+v", got)
	}
	if callerUser(decoded, 20) != nil {
		t.Fatal("callee and filler users must be dropped from the voip updates")
	}
	if callerUser(decoded, 3).GetPhoto() != nil {
		t.Fatal("caller photo must be stripped from the voip updates")
	}
}

func TestBuildCallErrorsWhenPayloadStillExceedsLimit(t *testing.T) {
	secret := bytes.Repeat([]byte{0x5a}, 256)
	// FirstName is kept in the slim caller. crypto/rand bytes do not gzip
	// well, so the sealed body stays over the APNs VoIP limit.
	noise := make([]byte, 20000)
	if _, err := rand.Read(noise); err != nil {
		t.Fatal(err)
	}
	user := mtproto.MakeTLUser(&mtproto.User{
		Id:         3,
		AccessHash: wrapperspb.Int64(99),
		FirstName:  wrapperspb.String(string(noise)),
		LastName:   wrapperspb.String("Lovelace"),
	}).To_User()
	updates := mtproto.MakeUpdatesByUpdatesUsers([]*mtproto.User{user},
		mtproto.MakeTLUpdatePhoneCall(&mtproto.Update{PhoneCall: requestedCall()}).To_Update())
	body, err := BuildCall(Call{
		UserID: 9, CallID: 100, AccessHash: 200, CallerID: 3, CallerName: "Ada Lovelace", Updates: updates,
	}, secret)
	if err == nil {
		t.Fatalf("expected over-limit error, got %d-byte payload", len(body))
	}
	if !strings.Contains(err.Error(), "voip payload is") || !strings.Contains(err.Error(), "limit 5120") {
		t.Fatalf("error %v", err)
	}
	if body != nil {
		t.Fatal("over-limit payload must not be returned")
	}
}

func mustDecodeUpdates(t *testing.T, encoded string) *mtproto.Updates {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeCallUpdates(raw)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
