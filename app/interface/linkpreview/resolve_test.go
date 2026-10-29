package linkpreview

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/teamgram/proto/mtproto"
)

func fixture() *Memory {
	m := NewMemory()
	m.PutPeer(storedPeer{
		ID: 42, Kind: KindUser, Title: "Ada Lovelace", Username: "adalov",
		About: "analyst", AvatarPath: "photos/ada.jpg", Verified: true,
	})
	m.PutPeer(storedPeer{
		ID: 7, Kind: KindBot, Title: "Helper", Username: "helperbot", About: "I help",
	})
	count := int32(1200)
	m.PutPeer(storedPeer{
		ID: 1073741824, Kind: KindChannel, Title: "News", Username: "newsroom",
		About: "daily", PhotoID: 99, Verified: true, MemberCount: &count, Broadcast: true,
	})
	gcount := int32(40)
	m.PutPeer(storedPeer{
		ID: 1073741900, Kind: KindGroup, Title: "Chat", Username: "chatroom",
		MemberCount: &gcount, Megagroup: true,
	})
	m.PutPhoto(99, "photos/news-m.jpg")
	m.PutMessage(1073741824, &mtproto.Message{
		Id: 15, Date: 1_700_000_000, Message: "hello channel",
		Media: &mtproto.MessageMedia{
			PredicateName:   mtproto.Predicate_messageMediaPhoto,
			Photo_FLAGPHOTO: &mtproto.Photo{Id: 99},
		},
		Views: wrapperspb.Int32(8),
	})
	m.PutMessage(1073741824, &mtproto.Message{
		Id: 16, Date: 1_700_000_100,
		Media: &mtproto.MessageMedia{
			PredicateName: mtproto.Predicate_messageMediaContact,
			PhoneNumber:   "15551212999",
			FirstName:     "Ada",
			LastName:      "Lovelace",
		},
	})
	m.PutMessage(1073741824, &mtproto.Message{
		Id: 14, Date: 1_600_000_000, Message: "older post",
	})
	m.PutInvite("Abcd1234efgh5678ijkl", storedInvite{
		Title: "Secret chat", About: "come in", MemberCount: 3, RequestNeeded: true, AvatarPath: "photos/inv.jpg",
	})
	m.PutStory(ownerUser, 42, &mtproto.StoryItem{
		Id: 4, Public: true, Date: 10, ExpireDate: int32(time.Now().Add(time.Hour).Unix()),
		Caption: wrapperspb.String("on the wire"),
		Media: &mtproto.MessageMedia{
			PredicateName:   mtproto.Predicate_messageMediaPhoto,
			Photo_FLAGPHOTO: &mtproto.Photo{Id: 99},
		},
	}, false)
	m.PutStory(ownerUser, 42, &mtproto.StoryItem{
		Id: 5, Date: 10, ExpireDate: int32(time.Now().Add(time.Hour).Unix()),
		Privacy: []*mtproto.PrivacyRule{{PredicateName: mtproto.Predicate_privacyValueAllowContacts}},
	}, false)
	m.PutPack(storedPack{ShortName: "Animals", Title: "Animals", Count: 12, AvatarPath: "stickers/animals.png"})
	m.PutPack(storedPack{ShortName: "Faces", Title: "Faces", Count: 4, Emoji: true})
	return m
}

func newSvc(m Catalog) *Service {
	return New(Config{
		LinkBaseURL:        "https://tell.cc",
		PublicMediaBaseURL: "https://cdn.tell.cc/media",
		MediaKey:           "test-media",
	}, m)
}

func TestResolvePublicPaths(t *testing.T) {
	svc := newSvc(fixture())
	ctx := context.Background()

	user, err := svc.Resolve(ctx, "/adalov", nil)
	if err != nil {
		t.Fatal(err)
	}
	if user.Kind != KindUser || user.Peer.Title != "Ada Lovelace" || !user.Peer.Verified || user.Peer.About != "analyst" {
		t.Fatalf("user: %#v", user.Peer)
	}
	if user.Peer.AvatarURL != "https://cdn.tell.cc/media/photos/ada.jpg" {
		t.Fatalf("avatar %s", user.Peer.AvatarURL)
	}
	if user.Link != "https://tell.cc/adalov" {
		t.Fatalf("link %s", user.Link)
	}

	bot, err := svc.Resolve(ctx, "https://tell.cc/helperbot?start=ref_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if bot.Kind != KindBot || bot.Peer.Start == nil || bot.Peer.Start.Param != "start" || bot.Peer.Start.Value != "ref_1" {
		t.Fatalf("bot %#v", bot)
	}

	ch, err := svc.Resolve(ctx, "/newsroom", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ch.Kind != KindChannel || ch.Peer.MemberCount == nil || *ch.Peer.MemberCount != 1200 || ch.Peer.AvatarURL != "https://cdn.tell.cc/media/photos/news-m.jpg" {
		t.Fatalf("channel %#v avatar %s", ch.Peer, ch.Peer.AvatarURL)
	}

	gr, err := svc.Resolve(ctx, "/chatroom", nil)
	if err != nil || gr.Kind != KindGroup || gr.Peer.Megagroup != true {
		t.Fatalf("group %#v %v", gr, err)
	}

	post, err := svc.Resolve(ctx, "/newsroom/15", nil)
	if err != nil {
		t.Fatal(err)
	}
	if post.Kind != KindMessage || post.Post.Excerpt != "hello channel" || post.Post.MediaType != "photo" || post.Post.MediaURL == "" || post.Post.Views == nil || *post.Post.Views != 8 {
		t.Fatalf("post %#v", post.Post)
	}

	web, err := svc.Resolve(ctx, "/s/newsroom/15", nil)
	if err != nil || web.Path != "/s/newsroom/15" {
		t.Fatalf("web post %#v %v", web, err)
	}
	cpost, err := svc.Resolve(ctx, "/c/1073741824/15", nil)
	if err != nil || cpost.Kind != KindMessage || cpost.Path != "/c/1073741824/15" {
		t.Fatalf("c post %#v %v", cpost, err)
	}

	feed, err := svc.Resolve(ctx, "/s/newsroom", nil)
	if err != nil {
		t.Fatal(err)
	}
	if feed.Kind != KindFeed || feed.Posts == nil || len(*feed.Posts) < 2 {
		t.Fatalf("feed %#v", feed.Posts)
	}
	if (*feed.Posts)[0].MessageID != 16 {
		t.Fatalf("newest first got %d", (*feed.Posts)[0].MessageID)
	}

	contact, err := svc.Resolve(ctx, "/newsroom/16", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(contact)
	if strings.Contains(string(raw), "15551212999") {
		t.Fatalf("phone leaked: %s", raw)
	}
	if contact.Post.Excerpt != "Ada Lovelace" || contact.Post.MediaType != "contact" {
		t.Fatalf("contact %#v", contact.Post)
	}

	inv, err := svc.Resolve(ctx, "/+Abcd1234efgh5678ijkl", nil)
	if err != nil || inv.Kind != KindInvite || inv.Invite.Title != "Secret chat" || inv.Invite.MemberCount != 3 || !inv.Invite.RequestNeeded {
		t.Fatalf("invite %#v %v", inv, err)
	}
	join, err := svc.Resolve(ctx, "/joinchat/Abcd1234efgh5678ijkl", nil)
	if err != nil || join.Invite.AvatarURL == "" {
		t.Fatalf("joinchat %#v %v", join, err)
	}

	st, err := svc.Resolve(ctx, "/adalov/s/4", nil)
	if err != nil || st.Kind != KindStory || st.Story.Caption != "on the wire" || st.Story.MediaURL == "" {
		t.Fatalf("story %#v %v", st, err)
	}
	if _, err = svc.Resolve(ctx, "/adalov/s/5", nil); errorCode(err) != ErrNotFound {
		t.Fatalf("private story: %v", err)
	}

	pack, err := svc.Resolve(ctx, "/addstickers/Animals", nil)
	if err != nil || pack.Kind != KindStickerSet || pack.Pack.Count != 12 || pack.Pack.AvatarURL == "" {
		t.Fatalf("stickers %#v %v", pack, err)
	}
	emoji, err := svc.Resolve(ctx, "/addemoji/Faces", nil)
	if err != nil || emoji.Kind != KindEmojiSet || !emoji.Pack.Emoji {
		t.Fatalf("emoji %#v %v", emoji, err)
	}

	px, err := svc.Resolve(ctx, "/proxy?server=proxy.tell.cc&port=443&secret=dd001122", nil)
	if err != nil || px.Kind != KindProxy || px.Proxy.Secret != "dd001122" || px.Proxy.Port != 443 {
		t.Fatalf("proxy %#v %v", px, err)
	}
	socks, err := svc.Resolve(ctx, "/socks", mapQuery("server", "10.0.0.1", "port", "1080", "user", "u", "pass", "p"))
	if err != nil || socks.Kind != KindSocks || socks.Socks.User != "u" || socks.Socks.Pass != "p" {
		t.Fatalf("socks %#v %v", socks, err)
	}
	share, err := svc.Resolve(ctx, "/share/url?url=https%3A%2F%2Fexample.com%2Fa&text=hi", nil)
	if err != nil || share.Kind != KindShare || share.Share.URL != "https://example.com/a" || share.Share.Text != "hi" {
		t.Fatalf("share %#v %v", share, err)
	}
	boost, err := svc.Resolve(ctx, "/boost/newsroom", nil)
	if err != nil || boost.Kind != KindBoost || boost.Peer == nil || boost.Peer.Username != "newsroom" {
		t.Fatalf("boost %#v %v", boost, err)
	}
	bare, err := svc.Resolve(ctx, "/boost", nil)
	if err != nil || bare.Kind != KindBoost || bare.Peer != nil {
		t.Fatalf("bare boost %#v %v", bare, err)
	}
	named, err := svc.Resolve(ctx, "/newsroom/boost", nil)
	if err != nil || named.Path != "/boost/newsroom" {
		t.Fatalf("name boost %#v %v", named, err)
	}
}

func TestPrivateAndInvalid(t *testing.T) {
	svc := newSvc(fixture())
	ctx := context.Background()
	cases := []string{"/missingname", "/c/999/1", "/s/adalov", "/adalov/15", "/boost/adalov", "/joinchat/nopehash1"}
	for _, path := range cases {
		_, err := svc.Resolve(ctx, path, nil)
		if errorCode(err) != ErrNotFound && errorCode(err) != ErrInvalidPath {
			t.Fatalf("%s -> %v", path, err)
		}
	}
	for _, path := range []string{"", "/proxy", "/share/url?url=javascript:alert(1)", "/../etc", "https://evil.example/ada"} {
		_, err := svc.Resolve(ctx, path, nil)
		if errorCode(err) != ErrInvalidPath {
			t.Fatalf("%s -> %v", path, err)
		}
	}
	if _, err := svc.Resolve(ctx, "/c/-1001073741824/15", nil); err != nil {
		t.Fatalf("telegram -100 form: %v", err)
	}
}

func TestRulesAndExcerpt(t *testing.T) {
	if !rulesArePublic("") || !rulesArePublic(`[{"predicate_name":"privacyValueAllowAll"}]`) {
		t.Fatal("allow all")
	}
	if rulesArePublic(`[{"predicate_name":"privacyValueDisallowAll"}]`) || rulesArePublic(`[{"predicate_name":"privacyValueAllowContacts"}]`) {
		t.Fatal("private rules treated as public")
	}
	if rulesArePublic("not-json") {
		t.Fatal("garbage should fail closed")
	}
	long := strings.Repeat("a", 200)
	got := excerptText(long)
	if len([]rune(got)) != excerptRunes {
		t.Fatalf("len %d", len([]rune(got)))
	}
	if pickPhotoFile([]sizeRow{{typ: "w", width: 1000, path: "big"}, {typ: "m", width: 320, path: "mid"}}) != "mid" {
		t.Fatal("prefer m")
	}
}

func TestQueriesOmitSecrets(t *testing.T) {
	blob := strings.ToLower(qUsername + qUser + qBot + qPrivacy + qChannel + qChat + qInvite + qMessage + qStory + qPack + qDocument)
	for _, bad := range []string{"phone", "token", "access_hash", "secret_key"} {
		if strings.Contains(blob, bad) {
			t.Fatalf("query contains %s", bad)
		}
	}
}

func TestHTTPTokenRateCacheAndMedia(t *testing.T) {
	mem := fixture()
	cat := &countCat{Memory: mem}
	cfg := Config{
		Token:                "s3cret",
		RatePerMin:           30,
		RateBurst:            30,
		CacheSeconds:         60,
		NotFoundCacheSeconds: 15,
		LinkBaseURL:          "https://tell.cc",
		PublicMediaBaseURL:   "https://cdn.tell.cc/media",
		MediaKey:             "media-key",
	}
	h := NewHandler(cfg, cat)
	srv := httptest.NewServer(h)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/linkpreview/v1/resolve?path=/adalov")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/linkpreview/v1/resolve?path=/adalov", nil)
	req.Header.Set(HeaderToken, "s3cret")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body := mustBody(t, res)
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Cache-Control"), "max-age=60") || !strings.Contains(res.Header.Get("Cache-Control"), "private") {
		t.Fatalf("headers %v body %s", res.Header, body)
	}
	etag := res.Header.Get("ETag")
	if etag == "" || !strings.Contains(body, `"kind":"user"`) {
		t.Fatalf("etag %s body %s", etag, body)
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/linkpreview/v1/resolve?path=/adalov", nil)
	req.Header.Set(HeaderToken, "s3cret")
	req.Header.Set("If-None-Match", etag)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotModified {
		t.Fatalf("etag status %d", res.StatusCode)
	}
	if cat.n.Load() != 1 {
		t.Fatalf("cache missed, lookups %d", cat.n.Load())
	}

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/linkpreview/v1/resolve?path=/notauser", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body = mustBody(t, res)
	if res.StatusCode != http.StatusNotFound || !strings.Contains(body, `"error":"not_found"`) {
		t.Fatalf("missing %d %s", res.StatusCode, body)
	}

	health, err := http.Get(srv.URL + "/linkpreview/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	health.Body.Close()
	if health.StatusCode != 200 {
		t.Fatalf("health %d", health.StatusCode)
	}

	limited := NewHandler(Config{RatePerMin: 2, RateBurst: 2, LinkBaseURL: "https://tell.cc"}, NewMemory())
	lsrv := httptest.NewServer(limited)
	defer lsrv.Close()
	var saw429 bool
	for i := 0; i < 5; i++ {
		res, err = http.Get(lsrv.URL + "/linkpreview/v1/resolve?path=/boost")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode == http.StatusTooManyRequests {
			if res.Header.Get("Retry-After") == "" {
				t.Fatal("missing Retry-After")
			}
			saw429 = true
			break
		}
	}
	if !saw429 {
		t.Fatal("rate limit did not trip")
	}

	mediaH := NewHandler(Config{MediaKey: "media-key", MediaTTLSeconds: 3600, LinkBaseURL: "https://tell.cc"}, NewMemory())
	msrv := httptest.NewServer(mediaH)
	defer msrv.Close()
	signed := mediaURL(mediaH.cfg, "photos/ada.jpg", time.Now())
	if !strings.HasPrefix(signed, "/linkpreview/v1/media?") {
		t.Fatalf("signed %s", signed)
	}
	res, err = http.Get(msrv.URL + signed)
	if err != nil {
		t.Fatal(err)
	}
	body = mustBody(t, res)
	if res.StatusCode != http.StatusNotFound || !strings.Contains(body, ErrMediaUnavailable) {
		t.Fatalf("media %d %s", res.StatusCode, body)
	}
	res, err = http.Get(msrv.URL + "/linkpreview/v1/media?p=photos/secret.jpg&exp=1&sig=00")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("bad sig %d", res.StatusCode)
	}
}

type countCat struct {
	*Memory
	n atomic.Int32
}

func (c *countCat) PeerByUsername(ctx context.Context, username string) (*storedPeer, error) {
	c.n.Add(1)
	return c.Memory.PeerByUsername(ctx, username)
}

func mapQuery(kv ...string) map[string][]string {
	q := map[string][]string{}
	for i := 0; i < len(kv); i += 2 {
		q[kv[i]] = []string{kv[i+1]}
	}
	return q
}

func mustBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	buf := make([]byte, 8192)
	n, _ := res.Body.Read(buf)
	return string(buf[:n])
}
