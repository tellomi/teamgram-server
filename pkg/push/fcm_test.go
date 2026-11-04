package push

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestLoadFCMDefaultOff(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	path := writeServiceAccount(t, key, "tellomi-test", "push@test.iam.gserviceaccount.com", "https://oauth.example/token")
	t.Setenv(EnvFCMEnabled, "")
	t.Setenv(EnvFCMCredentials, path)
	fcm, err := LoadFCM()
	if err != nil || fcm != nil {
		t.Fatalf("off flag returned %#v %v", fcm, err)
	}
	t.Setenv(EnvFCMEnabled, "true")
	fcm, err = LoadFCM()
	if err != nil || fcm != nil {
		t.Fatalf("non-1 flag returned %#v %v", fcm, err)
	}
	t.Setenv(EnvFCMEnabled, "1")
	t.Setenv(EnvFCMCredentials, "")
	if _, err := LoadFCM(); !errors.Is(err, ErrFCMNotConfigured) {
		t.Fatalf("missing path: %v", err)
	}
	t.Setenv(EnvFCMCredentials, filepath.Join(t.TempDir(), "missing.json"))
	if _, err := LoadFCM(); !errors.Is(err, ErrFCMNotConfigured) {
		t.Fatalf("missing file: %v", err)
	}
}

func TestLoadFCMReadsFileAndHidesKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	path := writeServiceAccount(t, key, "tellomi-test", "push@test.iam.gserviceaccount.com", "https://oauth.example/token")
	t.Setenv(EnvFCMEnabled, "1")
	t.Setenv(EnvFCMCredentials, path)
	t.Setenv(EnvFCMProject, "override-project")
	t.Setenv(EnvFCMHost, "https://fcm.example")
	t.Setenv(EnvFCMTokenURL, "https://oauth.example/override")
	fcm, err := LoadFCM()
	if err != nil {
		t.Fatal(err)
	}
	if fcm.projectID != "override-project" || fcm.host != "https://fcm.example" || fcm.tokenURL != "https://oauth.example/override" {
		t.Fatalf("%+v", fcm)
	}
	if fcm.client.Timeout != fcmHTTPTimeout {
		t.Fatalf("timeout %s", fcm.client.Timeout)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`{"private_key":"-----BEGIN PRIVATE KEY-----\nSECRETKEY\n-----END PRIVATE KEY-----\n"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvFCMCredentials, bad)
	t.Setenv(EnvFCMProject, "")
	_, err = LoadFCM()
	if !errors.Is(err, ErrFCMNotConfigured) || strings.Contains(err.Error(), "SECRETKEY") || strings.Contains(err.Error(), "PRIVATE KEY") {
		t.Fatalf("error leaked the key: %v", err)
	}
}

func TestFCMSendPostsHTTPV1(t *testing.T) {
	var tokenHits int
	var mu sync.Mutex
	var gotAuth string
	var gotBody []byte
	var gotPath string
	var assertion string
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokenHits++
			form, _ := io.ReadAll(r.Body)
			vals, _ := url.ParseQuery(string(form))
			if vals.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
				t.Errorf("grant %q", vals.Get("grant_type"))
			}
			assertion = vals.Get("assertion")
			writeAccessToken(w)
			return
		}
		mu.Lock()
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"projects/tellomi-test/messages/1"}`))
	}))
	defer srv.Close()
	fcm := NewFCM(srv.URL, srv.URL+"/token", "tellomi-test", "push@test.iam.gserviceaccount.com", key)
	payload := []byte(`{"loc_key":"MESSAGE_TEXT","loc_args":"[\"Ada\",\"ping\"]","body":"ping","p":"cipher"}`)
	if err := fcm.Send(context.Background(), "reg-1", payload); err != nil {
		t.Fatal(err)
	}
	if err := fcm.Send(context.Background(), "reg-1", payload); err != nil {
		t.Fatal(err)
	}
	if tokenHits != 1 {
		t.Fatalf("token fetches %d", tokenHits)
	}
	verifyFCMAssertion(t, &key.PublicKey, assertion, srv.URL+"/token", "push@test.iam.gserviceaccount.com")
	if gotAuth != "Bearer ya29.test" {
		t.Fatalf("auth %q", gotAuth)
	}
	if gotPath != "/v1/projects/tellomi-test/messages:send" {
		t.Fatalf("path %s", gotPath)
	}
	var body struct {
		Message struct {
			Token        string            `json:"token"`
			Data         map[string]string `json:"data"`
			Notification json.RawMessage   `json:"notification"`
			Android      struct {
				Priority string `json:"priority"`
				TTL      string `json:"ttl"`
			} `json:"android"`
		} `json:"message"`
	}
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatal(err)
	}
	if body.Message.Token != "reg-1" || body.Message.Data["loc_key"] != "MESSAGE_TEXT" || body.Message.Data["p"] != "cipher" {
		t.Fatalf("%s", gotBody)
	}
	if len(body.Message.Notification) != 0 {
		t.Fatalf("notification block %s", body.Message.Notification)
	}
	if body.Message.Android.Priority != "HIGH" || body.Message.Android.TTL != "86400s" {
		t.Fatalf("%s", gotBody)
	}

	callPayload := []byte(`{"loc_key":"PHONE_CALL_REQUEST","call_id":"7","p":"ring"}`)
	if err := fcm.Send(context.Background(), "reg-1", callPayload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatal(err)
	}
	if body.Message.Android.TTL != "60s" || body.Message.Data["loc_key"] != locKeyPhoneCallRequest {
		t.Fatalf("call body %s", gotBody)
	}
}

func TestFCMDropsOnlyDeadTokens(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		drop   bool
	}{
		{
			name:   "unregistered",
			status: 404,
			body:   `{"error":{"code":404,"message":"Requested entity was not found.","status":"NOT_FOUND","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`,
			drop:   true,
		},
		{
			name:   "bad registration token",
			status: 400,
			body:   `{"error":{"code":400,"message":"The registration token is not a valid FCM registration token","status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"INVALID_ARGUMENT"},{"@type":"type.googleapis.com/google.rpc.BadRequest","fieldViolations":[{"field":"message.token","description":"Invalid registration token"}]}]}}`,
			drop:   true,
		},
		{
			name:   "sender mismatch",
			status: 403,
			body:   `{"error":{"code":403,"message":"SenderId mismatch","status":"PERMISSION_DENIED","details":[{"errorCode":"SENDER_ID_MISMATCH"}]}}`,
			drop:   true,
		},
		{
			name:   "bad payload stays",
			status: 400,
			body:   `{"error":{"code":400,"message":"Request contains an invalid argument.","status":"INVALID_ARGUMENT","details":[{"errorCode":"INVALID_ARGUMENT"},{"fieldViolations":[{"field":"message.data","description":"data values must be strings"}]}]}}`,
			drop:   false,
		},
		{
			name:   "unavailable stays",
			status: 503,
			body:   `{"error":{"code":503,"message":"unavailable","status":"UNAVAILABLE"}}`,
			drop:   false,
		},
	}
	var mu sync.Mutex
	current := cases[0]
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			writeAccessToken(w)
			return
		}
		mu.Lock()
		status, body := current.status, current.body
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	fcm := NewFCM(srv.URL, srv.URL+"/token", "tellomi-test", "push@test.iam.gserviceaccount.com", key)
	payload := []byte(`{"loc_key":"MESSAGE_NOTEXT","body":"新消息"}`)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mu.Lock()
			current = tc
			mu.Unlock()
			err := fcm.Send(context.Background(), "reg-1", payload)
			var fe *FCMError
			if !errors.As(err, &fe) || fe.Drop() != tc.drop {
				t.Fatalf("drop=%v err=%v", tc.drop, err)
			}
		})
	}
}

func TestFCMTimeoutDoesNotDrop(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			writeAccessToken(w)
			return
		}
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	fcm := NewFCM(srv.URL, srv.URL+"/token", "tellomi-test", "push@test.iam.gserviceaccount.com", key)
	if fcm.client.Timeout != fcmHTTPTimeout {
		t.Fatalf("default timeout %s", fcm.client.Timeout)
	}
	fcm.client.Timeout = 200 * time.Millisecond
	start := time.Now()
	err = fcm.Send(context.Background(), "reg-1", []byte(`{"loc_key":"MESSAGE_NOTEXT"}`))
	if err == nil {
		t.Fatal("expected timeout")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("elapsed %s", elapsed)
	}
	var fe *FCMError
	if errors.As(err, &fe) && fe.Drop() {
		t.Fatal("timeout dropped the token")
	}
}

func TestDeliverDropsUnregisteredFCMToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			writeAccessToken(w)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Requested entity was not found.","status":"NOT_FOUND","details":[{"errorCode":"UNREGISTERED"}]}}`))
	}))
	defer srv.Close()
	store := &MemoryStore{}
	ctx := context.Background()
	_ = store.Upsert(ctx, Device{AuthKeyID: 10, UserID: 9, TokenType: TokenFCM, Token: "fcm-token", Secret: bytesRepeat(256)})
	svc := NewService(store, Config{})
	svc.SetFCM(NewFCM(srv.URL, srv.URL+"/token", "tellomi-test", "push@test.iam.gserviceaccount.com", key))
	svc.Deliver(ctx, 9, textUpdate())
	waitDevicesGone(t, store, 9)
}

func TestDeliverFCMHidesPreviewText(t *testing.T) {
	const phrase = "secret-phrase-xyz"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	bodies := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			writeAccessToken(w)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		bodies <- raw
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	store := &memSettings{peer: NotifySettings{Found: true, ShowPreviews: 0}}
	secret := bytesRepeat(256)
	_ = store.Upsert(context.Background(), Device{
		AuthKeyID: 10, UserID: 9, TokenType: TokenFCM, Token: "fcm-token", Secret: secret, LangCode: "zh-Hans",
	})
	svc := NewService(store, Config{})
	svc.SetFCM(NewFCM(srv.URL, srv.URL+"/token", "tellomi-test", "push@test.iam.gserviceaccount.com", key))
	svc.Deliver(context.Background(), 9, textUpdateBody(phrase))
	var raw []byte
	select {
	case raw = <-bodies:
	case <-time.After(3 * time.Second):
		t.Fatal("fcm request was not sent")
	}
	if strings.Contains(string(raw), phrase) {
		t.Fatalf("preview off included the message text: %s", raw)
	}
	var body struct {
		Message struct {
			Data         map[string]string `json:"data"`
			Notification json.RawMessage   `json:"notification"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Message.Notification) != 0 {
		t.Fatalf("notification %s", body.Message.Notification)
	}
	if body.Message.Data["loc_key"] != "MESSAGE_NOTEXT" || strings.Contains(body.Message.Data["body"], phrase) || strings.Contains(body.Message.Data["loc_args"], phrase) {
		t.Fatalf("%s", raw)
	}
	plain, err := DecryptNotification(secret, body.Message.Data["p"])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), phrase) {
		t.Fatalf("sealed payload included the text: %s", plain)
	}
}

func TestSlowFCMDoesNotBlockOtherChannels(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var startOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			writeAccessToken(w)
			return
		}
		startOnce.Do(func() { close(started) })
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
		close(finished)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(unblock)
	fcm := NewFCM(srv.URL, srv.URL+"/token", "tellomi-test", "push@test.iam.gserviceaccount.com", key)
	fcm.client.Timeout = 30 * time.Second

	apns, apnsSrv, got := newAPNSCapture(t, "app.tellomi.chat")
	defer apnsSrv.Close()
	store := &MemoryStore{}
	ctx := context.Background()
	secret := bytesRepeat(256)
	_ = store.Upsert(ctx, Device{AuthKeyID: 1, UserID: 9, TokenType: TokenFCM, Token: "fcm-token", Secret: secret})
	_ = store.Upsert(ctx, Device{AuthKeyID: 2, UserID: 9, TokenType: TokenXiaomi, Token: "mi-token", Secret: secret})
	_ = store.Upsert(ctx, Device{AuthKeyID: 3, UserID: 9, TokenType: TokenAPNS, Token: "apns-token", Secret: secret})
	xm := &captureXiaomi{}
	svc := NewService(store, Config{})
	svc.apns = apns
	svc.SetFCM(fcm)
	svc.SetXiaomi(xm)

	done := make(chan struct{})
	go func() {
		svc.Deliver(ctx, 9, textUpdate())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Deliver blocked on FCM")
	}
	xm.mu.Lock()
	token := xm.token
	xm.mu.Unlock()
	if token != "mi-token" {
		t.Fatalf("xiaomi token %q", token)
	}
	if got.path != "/3/device/apns-token" {
		t.Fatalf("apns path %s", got.path)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("fcm was not attempted")
	}
	unblock()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("fcm did not finish")
	}
}

func writeAccessToken(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"access_token":"ya29.test","expires_in":3600,"token_type":"Bearer"}`))
}

func verifyFCMAssertion(t *testing.T, pub *rsa.PublicKey, assertion, tokenURL, email string) {
	t.Helper()
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt parts %d", len(parts))
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatal(err)
	}
	claimJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimJSON, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["iss"] != email || claims["scope"] != fcmScope || claims["aud"] != tokenURL {
		t.Fatalf("%s", claimJSON)
	}
	if strings.Contains(assertion, "PRIVATE") {
		t.Fatal("assertion contains key material")
	}
}

func writeServiceAccount(t *testing.T, key *rsa.PrivateKey, project, email, tokenURI string) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	raw, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"project_id":   project,
		"private_key":  string(pemBytes),
		"client_email": email,
		"token_uri":    tokenURI,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitDevicesGone(t *testing.T, store Store, userID int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		left, err := store.ListByUser(context.Background(), userID)
		if err != nil {
			t.Fatal(err)
		}
		if len(left) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("dead token was kept")
}

func textUpdateBody(body string) *mtproto.Updates {
	user := mtproto.MakeTLUser(&mtproto.User{
		Id:        3,
		FirstName: &wrapperspb.StringValue{Value: "Ada"},
	}).To_User()
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Out: false, Id: 4, Message: body,
		FromId: mtproto.MakePeerUser(3), PeerId: mtproto.MakePeerUser(3),
	}).To_Message()
	return mtproto.MakeUpdatesByUpdatesUsers([]*mtproto.User{user},
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: msg}).To_Update())
}
