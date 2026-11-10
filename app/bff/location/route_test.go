package location

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAppleSearchAndCache(t *testing.T) {
	key, pemBytes := testAppleKey(t)
	var tokenHits, nearbyHits, searchHits int32
	var nearbyLoc, searchLoc, pageTok string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/token":
			atomic.AddInt32(&tokenHits, 1)
			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !jwtSignedBy(t, raw, key) {
				t.Errorf("jwt not signed by the map key")
			}
			_, _ = w.Write([]byte(`{"accessToken":"access-1","expiresInSeconds":1800}`))
		case "/v1/searchNearby":
			atomic.AddInt32(&nearbyHits, 1)
			if r.Header.Get("Authorization") != "Bearer access-1" {
				t.Errorf("nearby auth %q", r.Header.Get("Authorization"))
			}
			nearbyLoc = r.URL.Query().Get("searchLocation")
			_, _ = w.Write([]byte(`{"results":[{"id":"A1","name":"星巴克","poiCategory":"Cafe","formattedAddressLines":["建国路1号","东城区"],"coordinate":{"latitude":39.910104,"longitude":116.403744}}]}`))
		case "/v1/search":
			atomic.AddInt32(&searchHits, 1)
			searchLoc = r.URL.Query().Get("searchLocation")
			pageTok = r.URL.Query().Get("pageToken")
			if pageTok == "" {
				_, _ = w.Write([]byte(`{"results":[{"id":"A2","name":"咖啡店","poiCategory":"Cafe","formattedAddressLines":["路2"],"coordinate":{"latitude":39.92,"longitude":116.41}}],"pagination":{"nextPageToken":"p2"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"results":[{"id":"A3","name":"第二页","poiCategory":"Restaurant","formattedAddressLines":["路3"],"coordinate":{"latitude":39.93,"longitude":116.42}}]}`))
		default:
			t.Errorf("path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ctx := WithPlatform(WithClient(context.Background(), New(Options{
		AppleTeamID: "TEAMID1234", AppleKeyID: "KEYID12345", AppleKeyPEM: pemBytes, AppleBase: srv.URL,
		QPS: 100, Daily: 100, UserPerMin: 100,
	})), platformIOS)
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9087, Long: 116.3975}).To_InputGeoPoint()
	got, next := Search(ctx, 7, point, "", "")
	if len(got) != 1 || next != "" {
		t.Fatalf("nearby %d next %q", len(got), next)
	}
	msg := got[0].GetSendMessage()
	if msg.GetPredicateName() != mtproto.Predicate_botInlineMessageMediaVenue || msg.GetProvider() != "apple" || msg.GetVenueType() != "food/cafe" {
		t.Fatalf("venue %+v", msg)
	}
	if msg.GetTitle() != "星巴克" || msg.GetVenueId() != "A1" {
		t.Fatalf("title %q id %q", msg.GetTitle(), msg.GetVenueId())
	}
	if nearbyLoc != "39.908700,116.397500" {
		t.Fatalf("nearby location %s, want WGS-84", nearbyLoc)
	}
	if _, _ = Search(ctx, 7, point, "", ""); atomic.LoadInt32(&tokenHits) != 1 || atomic.LoadInt32(&nearbyHits) != 1 {
		t.Fatalf("cache token %d nearby %d", tokenHits, nearbyHits)
	}
	found, next := Search(ctx, 7, point, "咖啡", "")
	if len(found) != 1 || next != "p2" || found[0].GetId() != "A2" {
		t.Fatalf("search %d next %q id %s", len(found), next, found[0].GetId())
	}
	if searchLoc != "39.908700,116.397500" {
		t.Fatalf("search location %s", searchLoc)
	}
	page, next := Search(ctx, 7, point, "咖啡", "p2")
	if len(page) != 1 || page[0].GetId() != "A3" || next != "" || pageTok != "p2" {
		t.Fatalf("page %d next %q tok %q", len(page), next, pageTok)
	}
	if page[0].GetSendMessage().GetVenueType() != "food/default" {
		t.Fatal(page[0].GetSendMessage().GetVenueType())
	}
}

func TestPlatformRouting(t *testing.T) {
	_, pemBytes := testAppleKey(t)
	var appleHits, amapHits int32
	appleSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/token" {
			_, _ = w.Write([]byte(`{"accessToken":"t","expiresInSeconds":1800}`))
			return
		}
		atomic.AddInt32(&appleHits, 1)
		_, _ = w.Write([]byte(`{"results":[{"id":"P","name":"苹果店","poiCategory":"Store","coordinate":{"latitude":39.91,"longitude":116.40}}]}`))
	}))
	defer appleSrv.Close()
	amapSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&amapHits, 1)
		_, _ = w.Write([]byte(`{"status":"1","infocode":"10000","pois":[{"id":"B","name":"高德店","typecode":"0601","address":"路","location":"116.40,39.91","pname":"北京市","cityname":"北京市","adname":"东城区"}]}`))
	}))
	defer amapSrv.Close()
	opts := Options{
		AMapKey: "k", AMapBase: amapSrv.URL,
		AppleTeamID: "TEAMID1234", AppleKeyID: "KEYID12345", AppleKeyPEM: pemBytes, AppleBase: appleSrv.URL,
		QPS: 100, Daily: 100, UserPerMin: 100,
	}
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9087, Long: 116.3975}).To_InputGeoPoint()

	ios := WithPlatform(WithClient(context.Background(), New(opts)), platformIOS)
	got, _ := Search(ios, 1, point, "", "")
	if len(got) != 1 || got[0].GetSendMessage().GetProvider() != "apple" || atomic.LoadInt32(&amapHits) != 0 {
		t.Fatalf("ios provider %s amap hits %d", got[0].GetSendMessage().GetProvider(), amapHits)
	}

	android := WithPlatform(WithClient(context.Background(), New(opts)), platformAndroid)
	got, _ = Search(android, 1, point, "店", "")
	if len(got) != 1 || got[0].GetSendMessage().GetProvider() != "amap" {
		t.Fatalf("android %+v", got[0].GetSendMessage())
	}
	appleBefore := atomic.LoadInt32(&appleHits)

	unknown := WithPlatform(WithClient(context.Background(), New(opts)), platformUnknown)
	got, _ = Search(unknown, 2, point, "", "")
	if len(got) != 1 || got[0].GetSendMessage().GetProvider() != "apple" {
		t.Fatalf("unknown first %+v", got[0].GetSendMessage())
	}
	if atomic.LoadInt32(&appleHits) <= appleBefore {
		t.Fatal("unknown did not ask apple")
	}

	off := WithPlatform(WithClient(context.Background(), New(Options{QPS: 100, Daily: 100, UserPerMin: 100})), platformUnknown)
	if got, _ = Search(off, 3, point, "", ""); len(got) != 0 {
		t.Fatalf("off returned %d", len(got))
	}
}

func TestUnknownFallsThroughWhenAppleFails(t *testing.T) {
	_, pemBytes := testAppleKey(t)
	appleSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/token" {
			_, _ = w.Write([]byte(`{"accessToken":"t","expiresInSeconds":1800}`))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer appleSrv.Close()
	amapSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"1","infocode":"10000","pois":[{"id":"B","name":"高德店","typecode":"1001","address":"路","location":"116.40,39.91","pname":"北京市","cityname":"北京市","adname":"东城区"}]}`))
	}))
	defer amapSrv.Close()
	ctx := WithPlatform(WithClient(context.Background(), New(Options{
		AMapKey: "k", AMapBase: amapSrv.URL,
		AppleTeamID: "TEAMID1234", AppleKeyID: "KEYID12345", AppleKeyPEM: pemBytes, AppleBase: appleSrv.URL,
		QPS: 100, Daily: 100, UserPerMin: 100,
	})), platformUnknown)
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9, Long: 116.4}).To_InputGeoPoint()
	got, _ := Search(ctx, 1, point, "酒店", "")
	if len(got) != 1 || got[0].GetSendMessage().GetProvider() != "amap" || got[0].GetSendMessage().GetVenueType() != "travel/hotel" {
		t.Fatalf("%+v", got[0].GetSendMessage())
	}
}

func testAppleKey(t *testing.T) (*ecdsa.PrivateKey, []byte) {
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

func jwtSignedBy(t *testing.T, raw string, key *ecdsa.PrivateKey) bool {
	t.Helper()
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return false
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if json.Unmarshal(header, &h) != nil || h.Alg != "ES256" || h.Kid != "KEYID12345" {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var p struct {
		Iss   string `json:"iss"`
		Scope string `json:"scope"`
	}
	if json.Unmarshal(payload, &p) != nil || p.Iss != "TEAMID1234" || p.Scope != "server_api" {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return false
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	return ecdsa.Verify(&key.PublicKey, sum[:], r, s)
}

func TestScrubCoords(t *testing.T) {
	in := `Get "https://maps-api.apple.com/v1/search?searchLocation=39.908700,116.397500": timeout`
	if got := scrubCoords(in); strings.Contains(got, "39.908") || strings.Contains(got, "searchLocation") {
		t.Fatal(got)
	}
}
