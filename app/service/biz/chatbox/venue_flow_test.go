package chatbox

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/location"
)

func TestVenueInlineNearbySearchPageAndSend(t *testing.T) {
	amap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		n := 1
		if r.URL.Query().Get("page_num") == "" || r.URL.Query().Get("page_num") == "1" {
			if r.URL.Query().Get("keywords") == "" {
				n = 25
			}
		}
		var b []byte
		b = append(b, `{"status":"1","infocode":"10000","pois":[`...)
		for i := 0; i < n; i++ {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, []byte(fmt.Sprintf(`{"id":"B%d","name":"店%d","typecode":"050500","address":"路%d","location":"116.40,39.91","pname":"北京市","cityname":"北京市","adname":"东城区"}`, i, i, i))...)
		}
		b = append(b, `]}`...)
		_, _ = w.Write(b)
	}))
	defer amap.Close()
	key, pemBytes := flowAppleKey(t)
	apple := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/token" {
			_, _ = w.Write([]byte(`{"accessToken":"t","expiresInSeconds":1800}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"id":"A1","name":"苹果咖啡","poiCategory":"Cafe","formattedAddressLines":["建国路"],"coordinate":{"latitude":39.91,"longitude":116.40}}]}`))
	}))
	defer apple.Close()

	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9087, Long: 116.3975}).To_InputGeoPoint()
	android := location.WithPlatform(location.WithClient(context.Background(), location.New(location.Options{
		AMapKey: "k", AMapBase: amap.URL, QPS: 100, Daily: 100, UserPerMin: 100,
	})), "android")
	b := newBook(nil)
	nearby, err := b.InlineResults(android, 3, userIn(mtproto.BotVenueId), peerIn(9), "", "", point)
	if err != nil {
		t.Fatal(err)
	}
	if len(nearby.GetResults()) != 25 || nearby.GetNextOffset().GetValue() != "2" {
		t.Fatalf("nearby %d next %q", len(nearby.GetResults()), nearby.GetNextOffset().GetValue())
	}
	if nearby.GetResults()[0].GetSendMessage().GetPredicateName() != mtproto.Predicate_botInlineMessageMediaVenue {
		t.Fatal(nearby.GetResults()[0].GetSendMessage().GetPredicateName())
	}
	searched, err := b.InlineResults(android, 3, userIn(mtproto.BotVenueId), peerIn(9), "咖啡", "", point)
	if err != nil || len(searched.GetResults()) != 1 || searched.GetResults()[0].GetSendMessage().GetVenueType() != "food/cafe" {
		t.Fatalf("search %v len %d", err, len(searched.GetResults()))
	}
	page, err := b.InlineResults(android, 3, userIn(mtproto.BotVenueId), peerIn(9), "", "2", point)
	if err != nil || len(page.GetResults()) != 1 || page.GetNextOffset().GetValue() != "" {
		t.Fatalf("page %v len %d next %q", err, len(page.GetResults()), page.GetNextOffset().GetValue())
	}
	req, ready, err := b.TakeInlineResult(android, 3, peerIn(9), nearby.GetQueryId(), nearby.GetResults()[0].GetId(), 11, false, false, false, 0, nil)
	if err != nil || ready != nil {
		t.Fatalf("send %v %v", err, ready)
	}
	media := req.Message.GetMedia()
	if media.GetPredicateName() != mtproto.Predicate_messageMediaVenue || media.GetProvider() != "amap" || media.GetTitle() != "店0" {
		t.Fatalf("media %+v", media)
	}
	if media.GetGeo().GetPredicateName() != mtproto.Predicate_geoPoint || media.GetGeo().GetAccessHash() == 0 {
		t.Fatal("geo")
	}

	ios := location.WithPlatform(location.WithClient(context.Background(), location.New(location.Options{
		AppleTeamID: "TEAMID1234", AppleKeyID: "KEYID12345", AppleKeyPEM: pemBytes, AppleBase: apple.URL,
		QPS: 100, Daily: 100, UserPerMin: 100,
	})), "ios")
	iosRows, err := b.InlineResults(ios, 3, userIn(mtproto.BotVenueId), peerIn(9), "", "", point)
	if err != nil || len(iosRows.GetResults()) != 1 || iosRows.GetResults()[0].GetSendMessage().GetProvider() != "apple" {
		t.Fatalf("ios %v len %d", err, len(iosRows.GetResults()))
	}
	iosSend, _, err := b.TakeInlineResult(ios, 3, peerIn(9), iosRows.GetQueryId(), "A1", 12, false, false, false, 0, nil)
	if err != nil || iosSend.Message.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaVenue {
		t.Fatalf("ios send %v", err)
	}

	off := location.WithClient(context.Background(), location.New(location.Options{}))
	empty, err := b.InlineResults(off, 3, userIn(mtproto.BotVenueId), peerIn(9), "", "", point)
	if err != nil || len(empty.GetResults()) != 0 || empty.GetQueryId() == 0 {
		t.Fatalf("off err %v results %d query %d", err, len(empty.GetResults()), empty.GetQueryId())
	}
	_ = key
}

func flowAppleKey(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}
