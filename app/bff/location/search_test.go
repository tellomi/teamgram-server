package location

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

const fivePOI = `{"status":"1","infocode":"10000","pois":[
	{"id":"B1","name":"甲","typecode":"0501","address":"路1","location":"116.40,39.91","pname":"北京市","cityname":"北京市","adname":"东城区"},
	{"id":"B2","name":"乙","typecode":"0601","address":"路2","location":"116.41,39.92","pname":"北京市","cityname":"北京市","adname":"东城区"},
	{"id":"B3","name":"丙","typecode":"1001","address":"路3","location":"116.42,39.93","pname":"北京市","cityname":"北京市","adname":"东城区"},
	{"id":"B4","name":"丁","typecode":"1505","address":"路4","location":"116.43,39.94","pname":"北京市","cityname":"北京市","adname":"东城区"},
	{"id":"B5","name":"戊","typecode":"1301","address":"路5","location":"116.44,39.95","pname":"北京市","cityname":"北京市","adname":"东城区"}
]}`

func TestSearchAMapVenue(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("key %q", r.URL.Query().Get("key"))
		}
		if r.URL.Path != "/v5/place/around" {
			t.Errorf("path %s", r.URL.Path)
		}
		loc := r.URL.Query().Get("location")
		if len(loc) < 7 || loc[:7] != "116.403" {
			t.Errorf("location %s, want GCJ-02", loc)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"1","infocode":"10000","pois":[
			{"id":"B001","name":"星巴克","typecode":"050301","address":"建国路1号","location":"116.403744,39.910104","pname":"北京市","cityname":"北京市","adname":"东城区"},
			{"id":"B002","name":"二","typecode":"0601","address":"路2","location":"116.404000,39.911000","pname":"北京市","cityname":"北京市","adname":"东城区"},
			{"id":"B003","name":"三","typecode":"1001","address":"路3","location":"116.405000,39.912000","pname":"北京市","cityname":"北京市","adname":"东城区"},
			{"id":"B004","name":"四","typecode":"1505","address":"路4","location":"116.406000,39.913000","pname":"北京市","cityname":"北京市","adname":"东城区"},
			{"id":"B005","name":"五","typecode":"1301","address":"路5","location":"116.407000,39.914000","pname":"北京市","cityname":"北京市","adname":"东城区"}
		]}`))
	}))
	defer srv.Close()
	ctx := WithClient(context.Background(), New(Options{
		AMapKey: "test-key", AMapBase: srv.URL, QPS: 100, Daily: 100, UserPerMin: 100,
	}))
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9087, Long: 116.3975}).To_InputGeoPoint()
	got, next := Search(ctx, 7, point, "星巴克", "")
	if len(got) != 5 || next != "" {
		t.Fatalf("results %d next %q", len(got), next)
	}
	hit := got[0]
	if hit.GetType() != "venue" || hit.GetId() != "B001" {
		t.Fatalf("result %+v", hit)
	}
	msg := hit.GetSendMessage()
	if msg.GetPredicateName() != mtproto.Predicate_botInlineMessageMediaVenue {
		t.Fatalf("send %s", msg.GetPredicateName())
	}
	if msg.GetProvider() != "amap" || msg.GetVenueId() != "B001" || msg.GetVenueType() != "food/default" {
		t.Fatalf("venue provider=%s id=%s type=%s", msg.GetProvider(), msg.GetVenueId(), msg.GetVenueType())
	}
	if msg.GetTitle() != "星巴克" || msg.GetAddress() != "北京市东城区建国路1号" {
		t.Fatalf("title %q address %q", msg.GetTitle(), msg.GetAddress())
	}
	if msg.GetGeo().GetAccessHash() == 0 {
		t.Fatal("access_hash")
	}
	if _, _ = Search(ctx, 7, point, "星巴克", ""); atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("cache missed, hits %d", hits)
	}
}

func TestSearchHereAndNearby(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v3/geocode/regeo":
			_, _ = w.Write([]byte(`{"status":"1","infocode":"10000","regeocode":{"formatted_address":"北京市东城区东长安街","addressComponent":{"adcode":"110101","province":"北京市","city":[],"district":"东城区"}}}`))
		default:
			_, _ = w.Write([]byte(fivePOI))
		}
	}))
	defer srv.Close()
	ctx := WithClient(context.Background(), New(Options{
		AMapKey: "k", AMapBase: srv.URL, QPS: 100, Daily: 100, UserPerMin: 100,
	}))
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9087, Long: 116.3975}).To_InputGeoPoint()
	got, next := Search(ctx, 1, point, "", "")
	if len(got) != 6 || got[0].GetSendMessage().GetTitle() != "当前位置" {
		t.Fatalf("len %d title %q", len(got), got[0].GetSendMessage().GetTitle())
	}
	if got[0].GetSendMessage().GetProvider() != "amap" || got[0].GetSendMessage().GetGeo().GetLat() != 39.9087 {
		t.Fatalf("here %+v", got[0].GetSendMessage())
	}
	if next != "" {
		t.Fatalf("next %q", next)
	}
	if got[1].GetSendMessage().GetVenueType() != "food/default" {
		t.Fatal(got[1].GetSendMessage().GetVenueType())
	}
}

func TestSearchEmptyWithoutKeyOrOutsideChina(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9087, Long: 116.3975}).To_InputGeoPoint()
	ctx := WithClient(context.Background(), New(Options{
		TencentKey: "tencent-only", AMapBase: srv.URL, TencentBase: srv.URL, QPS: 100, Daily: 100, UserPerMin: 100,
	}))
	if got, _ := Search(ctx, 1, point, "", ""); len(got) != 0 || hits != 0 {
		t.Fatalf("unset amap key returned %d hits %d", len(got), hits)
	}
	ctx = WithClient(context.Background(), New(Options{
		AMapKey: "k", AMapBase: srv.URL, QPS: 100, Daily: 100, UserPerMin: 100,
	}))
	nyc := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 40.7128, Long: -74.006}).To_InputGeoPoint()
	if got, _ := Search(ctx, 1, nyc, "cafe", ""); len(got) != 0 || atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("outside china %d hits %d", len(got), hits)
	}
	if got, _ := Search(ctx, 1, nil, "cafe", ""); len(got) != 0 {
		t.Fatal("missing point")
	}
}

func TestSearchFailureIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	ctx := WithClient(context.Background(), New(Options{
		AMapKey: "k", AMapBase: srv.URL, QPS: 100, Daily: 100, UserPerMin: 100,
	}))
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9, Long: 116.4}).To_InputGeoPoint()
	got, _ := Search(ctx, 1, point, "咖啡", "")
	if len(got) != 0 {
		t.Fatalf("got %d", len(got))
	}
}

func TestSearchTencentFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v5/place/around" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"data":[{"id":"T1","title":"腾讯点","address":"中关村","category":"美食:咖啡","location":{"lat":39.91,"lng":116.40},"ad_info":{"province":"北京市","city":"北京市","district":"海淀区"}}]}`))
	}))
	defer srv.Close()
	ctx := WithClient(context.Background(), New(Options{
		AMapKey: "k", TencentKey: "tk", AMapBase: srv.URL, TencentBase: srv.URL,
		QPS: 100, Daily: 100, UserPerMin: 100,
	}))
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9, Long: 116.4}).To_InputGeoPoint()
	got, _ := Search(ctx, 3, point, "咖啡", "")
	if len(got) != 1 || got[0].GetSendMessage().GetProvider() != "tencent" || got[0].GetType() != "venue" {
		t.Fatalf("%d provider %s", len(got), got[0].GetSendMessage().GetProvider())
	}
	if got[0].GetSendMessage().GetVenueType() != "food/default" {
		t.Fatal(got[0].GetSendMessage().GetVenueType())
	}
}

func TestSearchQuotaOpensBreaker(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"status":"0","infocode":"10003","info":"DAILY_QUERY_OVER_LIMIT"}`))
	}))
	defer srv.Close()
	ctx := WithClient(context.Background(), New(Options{
		AMapKey: "k", AMapBase: srv.URL, QPS: 100, Daily: 100, UserPerMin: 100,
	}))
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9, Long: 116.4}).To_InputGeoPoint()
	if got, _ := Search(ctx, 1, point, "咖啡", ""); len(got) != 0 {
		t.Fatal("quota returned rows")
	}
	if got, _ := Search(ctx, 1, point, "咖啡", ""); len(got) != 0 {
		t.Fatal("breaker returned rows")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits %d", hits)
	}
}

func TestSearchRateLimit(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(fivePOI))
	}))
	defer srv.Close()
	ctx := WithClient(context.Background(), New(Options{
		AMapKey: "k", AMapBase: srv.URL, QPS: 100, Daily: 100, UserPerMin: 1,
	}))
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 31.2, Long: 121.5}).To_InputGeoPoint()
	Search(ctx, 9, point, "甲", "")
	Search(ctx, 9, point, "乙", "")
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits %d", hits)
	}
}

func TestSearchDailyBudget(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(fivePOI))
	}))
	defer srv.Close()
	ctx := WithClient(context.Background(), New(Options{
		AMapKey: "k", AMapBase: srv.URL, QPS: 100, Daily: 1, UserPerMin: 100,
	}))
	a := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 31.2, Long: 121.5}).To_InputGeoPoint()
	b := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 31.3, Long: 121.6}).To_InputGeoPoint()
	Search(ctx, 1, a, "甲", "")
	Search(ctx, 2, b, "乙", "")
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits %d", hits)
	}
}

func TestSearchSingleFlight(t *testing.T) {
	var hits int32
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			close(started)
		}
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(fivePOI))
	}))
	defer srv.Close()
	ctx := WithClient(context.Background(), New(Options{
		AMapKey: "k", AMapBase: srv.URL, QPS: 100, Daily: 100, UserPerMin: 100,
	}))
	point := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 39.9, Long: 116.4}).To_InputGeoPoint()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); Search(ctx, 1, point, "同", "") }()
	<-started
	go func() { defer wg.Done(); Search(ctx, 1, point, "同", "") }()
	wg.Wait()
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits %d", hits)
	}
}

func TestStaticMapCached(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3}
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Path != "/v3/staticmap" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write(png)
	}))
	defer srv.Close()
	ctx := WithClient(context.Background(), New(Options{
		AMapKey: "k", AMapBase: srv.URL, QPS: 100, Daily: 100, UserPerMin: 100,
	}))
	body, ok := StaticMap(ctx, 4, 39.9087, 116.3975, 2000, 0, 30, 9)
	if !ok || len(body) != len(png) {
		t.Fatalf("ok %v len %d", ok, len(body))
	}
	if _, ok = StaticMap(ctx, 4, 39.9087, 116.3975, 2000, 0, 30, 9); !ok || atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("cache hits %d", hits)
	}
	empty := WithClient(context.Background(), New(Options{TencentKey: "only"}))
	if _, ok = StaticMap(empty, 4, 39.9, 116.4, 100, 100, 10, 1); ok {
		t.Fatal("static map without amap key")
	}
}

func TestRedactAndSign(t *testing.T) {
	if got := redact("https://restapi.amap.com/v3?key=sekret&sig=sigsek", "sekret", "sigsek", "tk"); got != "https://restapi.amap.com/v3?key=redacted&sig=redacted" {
		t.Fatal(got)
	}
	raw := "https://restapi.amap.com/v5/place/text?keywords=%E5%92%96%E5%95%A1&key=testkey"
	signed, err := signAMap(raw, "secret")
	if err != nil {
		t.Fatal(err)
	}
	// Canonical string is the decoded values, sorted by name, plus the secret.
	sum := md5.Sum([]byte("key=testkey&keywords=咖啡secret"))
	want := hex.EncodeToString(sum[:])
	if !containsSig(signed, want) {
		t.Fatal(signed)
	}
}

func containsSig(raw, sig string) bool {
	return len(raw) > len(sig) && raw[len(raw)-len(sig):] == sig
}
