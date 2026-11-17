package retention

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)

func daysAgo(d int) int64 { return now.Add(-time.Duration(d) * 24 * time.Hour).Unix() }

func cfg() Config { c := Defaults(); c.Enabled = true; c.DryRun = false; return c }

func TestFromEnv(t *testing.T) {
	t.Setenv(EnvEnabled, "true")
	t.Setenv(EnvDays, "30")
	t.Setenv(EnvGraceDays, "7")
	t.Setenv(EnvDryRun, "1")
	t.Setenv(EnvBatch, "10")
	t.Setenv(EnvMaxPerRun, "0")
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Enabled || !c.DryRun || c.Days != 30 || c.GraceDays != 7 || c.Batch != 10 || c.MaxPerRun != 0 {
		t.Fatalf("got %+v", c)
	}
	t.Setenv(EnvDays, "x")
	if _, err := FromEnv(); err == nil {
		t.Fatal("want error for bad days")
	}
	t.Setenv(EnvDays, "0")
	if _, err := FromEnv(); err == nil {
		t.Fatal("want error for zero days")
	}
}

func TestFromEnvDefaultsOff(t *testing.T) {
	for _, k := range []string{EnvEnabled, EnvDays, EnvGraceDays, EnvDryRun, EnvBatch, EnvMaxPerRun, EnvRunHour} {
		t.Setenv(k, "")
	}
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.Enabled || !c.DryRun || c.Days != 90 || c.GraceDays != 30 {
		t.Fatalf("got %+v", c)
	}
}

func TestProtects(t *testing.T) {
	grace := 30 * 24 * time.Hour
	cases := []struct {
		name string
		sub  Subscription
		want bool
	}{
		{"free", Subscription{}, false},
		{"lifetime premium", Subscription{Premium: true}, true},
		{"active", Subscription{Premium: true, ExpireDate: daysAgo(-10)}, true},
		{"lapsed inside grace", Subscription{Premium: true, ExpireDate: daysAgo(29)}, true},
		{"flag cleared inside grace", Subscription{ExpireDate: daysAgo(5)}, true},
		{"lapsed past grace", Subscription{Premium: true, ExpireDate: daysAgo(31)}, false},
		{"flag cleared past grace", Subscription{ExpireDate: daysAgo(40)}, false},
	}
	for _, c := range cases {
		if got := c.sub.Protects(now, grace); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestADR0083RetentionExemptions(t *testing.T) {
	cases := []Media{
		{Kind: KindDocument, MimeType: "image/gif"},
		{Kind: KindDocument, MimeType: "video/mp4", Attributes: `[{"predicate_name":"documentAttributeAnimated"}]`},
		{Kind: KindDocument, MimeType: "image/webp", Attributes: `[{"predicate_name":"documentAttributeSticker"}]`},
		{Kind: KindDocument, MimeType: "application/x-tgsticker"},
		{Kind: KindDocument, MimeType: "image/png", Attributes: `[{"predicate_name":"documentAttributeCustomEmoji"}]`},
		{Kind: KindDocument, MimeType: "application/x-tgwallpattern"},
		{Kind: KindPhoto, SizeTypes: []string{"a", "b", "c"}},
	}
	for i, m := range cases {
		if !Exempt(m) {
			t.Errorf("case %d %+v is not exempt", i, m)
		}
	}
	if !(Subscription{Premium: true}).Protects(now, 30*24*time.Hour) {
		t.Fatal("active subscriber is not protected")
	}
	t.Setenv(EnvDryRun, "true")
	t.Setenv(EnvEnabled, "true")
	c, err := FromEnv()
	if err != nil || !c.DryRun {
		t.Fatalf("dry-run config %+v %v", c, err)
	}
}

func TestExempt(t *testing.T) {
	cases := []struct {
		m    Media
		want bool
	}{
		{Media{Kind: KindDocument, MimeType: "video/mp4"}, false},
		{Media{Kind: KindDocument, MimeType: "audio/ogg", Attributes: `[{"predicate_name":"documentAttributeAudio","voice":true}]`}, false},
		{Media{Kind: KindDocument, MimeType: "image/gif"}, true},
		{Media{Kind: KindDocument, MimeType: "video/mp4", Attributes: `[{"predicate_name":"documentAttributeAnimated"}]`}, true},
		{Media{Kind: KindDocument, MimeType: "image/webp", Attributes: `[{"predicate_name":"documentAttributeSticker"}]`}, true},
		{Media{Kind: KindDocument, MimeType: "application/x-tgsticker"}, true},
		{Media{Kind: KindDocument, MimeType: "image/png", Attributes: `[{"predicate_name":"documentAttributeCustomEmoji"}]`}, true},
		{Media{Kind: KindDocument, MimeType: "application/x-tgwallpattern"}, true},
		{Media{Kind: KindPhoto, SizeTypes: []string{"s", "m", "x"}}, false},
		{Media{Kind: KindPhoto, SizeTypes: []string{"a", "b", "c"}}, true},
	}
	for i, c := range cases {
		if got := Exempt(c.m); got != c.want {
			t.Errorf("case %d %+v: got %v want %v", i, c.m, got, c.want)
		}
	}
}

func TestDecide(t *testing.T) {
	c := cfg()
	old := Media{Kind: KindDocument, ID: 1, UploadedAt: daysAgo(91), MimeType: "video/mp4"}
	young := old
	young.UploadedAt = daysAgo(89)
	free := []Subscription{{UserID: 1}, {UserID: 2}}
	mixed := []Subscription{{UserID: 1}, {UserID: 2, Premium: true, ExpireDate: daysAgo(-30)}}
	grace := []Subscription{{UserID: 1}, {UserID: 2, ExpireDate: daysAgo(10)}}
	pastGrace := []Subscription{{UserID: 1}, {UserID: 2, ExpireDate: daysAgo(31)}}
	gif := old
	gif.MimeType = "image/gif"
	cases := []struct {
		name    string
		m       Media
		holders []Subscription
		want    Reason
	}{
		{"young", young, free, ReasonYoung},
		{"free chat", old, free, ReasonExpire},
		{"mixed free/paid chat", old, mixed, ReasonSubscriber},
		{"grace", old, grace, ReasonSubscriber},
		{"past grace", old, pastGrace, ReasonExpire},
		{"exempt gif", gif, free, ReasonExempt},
		{"unreferenced", old, nil, ReasonUnreferenced},
	}
	for _, tc := range cases {
		if got := Decide(tc.m, tc.holders, now, c); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.name, got, tc.want)
		}
	}
}

func TestObjectsKeepThumbnails(t *testing.T) {
	got := Objects(Media{Kind: KindPhoto, ID: 7})
	want := [][2]string{{"photos", "0/7.dat"}, {"photos", "m/7.dat"}, {"photos", "x/7.dat"}, {"photos", "y/7.dat"}, {"photos", "w/7.dat"}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	for _, o := range got {
		if o[1] == "s/7.dat" || o[1] == "i/7.dat" {
			t.Fatalf("thumbnail removed: %v", o)
		}
	}
	if d := Objects(Media{Kind: KindDocument, ID: 9}); len(d) != 1 || d[0] != [2]string{"documents", "9.dat"} {
		t.Fatalf("document objects %v", d)
	}
	if !FullPhotoSize("x") || FullPhotoSize("s") || FullPhotoSize("i") {
		t.Fatal("FullPhotoSize")
	}
}

func TestNextRun(t *testing.T) {
	loc := time.FixedZone("x", 8*3600)
	n := time.Date(2026, 11, 2, 3, 0, 0, 0, loc)
	if got := NextRun(n, 4); !got.Equal(time.Date(2026, 11, 2, 4, 0, 0, 0, loc)) {
		t.Fatal(got)
	}
	if got := NextRun(n, 3); !got.Equal(time.Date(2026, 11, 3, 3, 0, 0, 0, loc)) {
		t.Fatal(got)
	}
}

// fakeStore models a small world of users, chats and media.
type fakeStore struct {
	media    []Media
	boxes    map[int64][]Holder // media id -> message boxes
	chats    map[int64][]int64
	channels map[int64][]int64          // channel id -> members
	chMedia  map[Kind]map[int64][]int64 // media -> channel ids
	users    map[int64]Subscription
	expired  map[int64]int64
	failHold map[int64]bool
}

func (f *fakeStore) ListCandidates(_ context.Context, kind Kind, cutoff, after int64, limit int) ([]Media, error) {
	var out []Media
	ms := append([]Media(nil), f.media...)
	sort.Slice(ms, func(i, j int) bool { return ms[i].ID < ms[j].ID })
	for _, m := range ms {
		if m.Kind == kind && m.ID > after && m.UploadedAt < cutoff && f.expired[m.ID] == 0 {
			out = append(out, m)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}
func (f *fakeStore) MessageHolders(_ context.Context, _ Kind, id int64) ([]Holder, error) {
	if f.failHold[id] {
		return nil, errors.New("boom")
	}
	return f.boxes[id], nil
}
func (f *fakeStore) ChatMembers(_ context.Context, id int64) ([]int64, error) {
	return f.chats[id], nil
}
func (f *fakeStore) ChannelMembers(_ context.Context, id int64) ([]int64, error) {
	return f.channels[id], nil
}
func (f *fakeStore) ChannelMedia(context.Context) (map[Kind]map[int64][]int64, error) {
	if f.chMedia == nil {
		return map[Kind]map[int64][]int64{}, nil
	}
	return f.chMedia, nil
}
func (f *fakeStore) Subscriptions(_ context.Context, ids []int64) ([]Subscription, error) {
	var out []Subscription
	for _, id := range ids {
		if s, ok := f.users[id]; ok {
			s.UserID = id
			out = append(out, s)
		}
	}
	return out, nil
}
func (f *fakeStore) MarkExpired(_ context.Context, _ Kind, id, at int64) error {
	f.expired[id] = at
	return nil
}
func (f *fakeStore) ExpiredAt(_ context.Context, _ Kind, id int64) (int64, error) {
	return f.expired[id], nil
}
func (f *fakeStore) CountOld(context.Context, Kind, int64) (int64, error) { return 2, nil }

type fakeDeleter struct{ keys []string }

func (d *fakeDeleter) Delete(_ context.Context, class, path string) error {
	d.keys = append(d.keys, class+"/"+path)
	return nil
}

// world: alice(1) free, bob(2) premium, carol(3) lapsed 10 days ago (grace),
// dave(4) lapsed 60 days ago.
func world() *fakeStore {
	old := daysAgo(100)
	return &fakeStore{
		media: []Media{
			{Kind: KindDocument, ID: 10, UploadedAt: old, Size: 100, MimeType: "video/mp4"},          // alice<->dave private: expire
			{Kind: KindDocument, ID: 11, UploadedAt: old, Size: 100, MimeType: "audio/ogg"},          // alice<->bob private: keep
			{Kind: KindDocument, ID: 12, UploadedAt: old, Size: 100, MimeType: "application/pdf"},    // group alice,dave,bob: keep
			{Kind: KindDocument, ID: 13, UploadedAt: old, Size: 100, MimeType: "image/gif"},          // exempt
			{Kind: KindDocument, ID: 14, UploadedAt: daysAgo(10), Size: 100, MimeType: "video/mp4"},  // young
			{Kind: KindDocument, ID: 15, UploadedAt: old, Size: 100, MimeType: "video/mp4"},          // forwarded from alice<->dave into chat with carol (grace): keep
			{Kind: KindDocument, ID: 16, UploadedAt: old, Size: 100, MimeType: "video/mp4"},          // channel with bob: keep
			{Kind: KindDocument, ID: 17, UploadedAt: old, Size: 100, MimeType: "video/mp4"},          // no references: keep
			{Kind: KindPhoto, ID: 20, UploadedAt: old, Size: 50, SizeTypes: []string{"s", "m", "x"}}, // alice<->dave photo: expire
			{Kind: KindPhoto, ID: 21, UploadedAt: old, Size: 50, SizeTypes: []string{"a", "b", "c"}}, // avatar: exempt
			{Kind: KindDocument, ID: 18, UploadedAt: old, Size: 100, MimeType: "video/mp4"},          // forwarded old media between free users: expire
		},
		boxes: map[int64][]Holder{
			10: {{1, peerUser, 4}, {4, peerUser, 1}},
			11: {{1, peerUser, 2}, {2, peerUser, 1}},
			12: {{1, peerChat, 100}},
			13: {{1, peerUser, 4}},
			15: {{1, peerUser, 4}, {4, peerUser, 1}, {3, peerUser, 1}, {1, peerUser, 3}},
			18: {{1, peerUser, 4}, {4, peerUser, 1}, {4, peerUser, 1}},
			20: {{1, peerUser, 4}, {4, peerUser, 1}},
			21: {{1, peerUser, 4}},
		},
		chats:    map[int64][]int64{100: {1, 4, 2}},
		channels: map[int64][]int64{500: {1, 2}},
		chMedia:  map[Kind]map[int64][]int64{KindDocument: {16: {500}}},
		users: map[int64]Subscription{
			1: {},
			2: {Premium: true, ExpireDate: daysAgo(-100)},
			3: {ExpireDate: daysAgo(10)},
			4: {Premium: true, ExpireDate: daysAgo(60)},
		},
		expired: map[int64]int64{},
	}
}

func TestSweeperDryRunChangesNothing(t *testing.T) {
	st, del := world(), &fakeDeleter{}
	c := cfg()
	c.DryRun = true
	rep, err := (&Sweeper{Cfg: c, Store: st, Objects: del}).Run(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(del.keys) != 0 || len(st.expired) != 0 {
		t.Fatalf("dry run changed state: %v %v", del.keys, st.expired)
	}
	if rep.Expired[KindDocument] != 2 || rep.Expired[KindPhoto] != 1 {
		t.Fatalf("report %s", rep)
	}
	if rep.EncryptedOld != 2 {
		t.Fatalf("encrypted %d", rep.EncryptedOld)
	}
	t.Log(rep.String())
}

func TestSweeperLive(t *testing.T) {
	st, del := world(), &fakeDeleter{}
	c := cfg()
	c.Batch = 2 // exercise paging
	rep, err := (&Sweeper{Cfg: c, Store: st, Objects: del}).Run(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for id := range st.expired {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) != 3 || ids[0] != 10 || ids[1] != 18 || ids[2] != 20 {
		t.Fatalf("expired %v; report %s", ids, rep)
	}
	sort.Strings(del.keys)
	want := []string{"documents/10.dat", "documents/18.dat", "photos/0/20.dat", "photos/m/20.dat", "photos/w/20.dat", "photos/x/20.dat", "photos/y/20.dat"}
	if len(del.keys) != len(want) {
		t.Fatalf("deleted %v", del.keys)
	}
	for i := range want {
		if del.keys[i] != want[i] {
			t.Fatalf("deleted %v want %v", del.keys, want)
		}
	}
	if rep.Reasons[ReasonSubscriber] != 4 || rep.Reasons[ReasonExempt] != 2 || rep.Reasons[ReasonUnreferenced] != 1 {
		t.Fatalf("reasons %v", rep.Reasons)
	}
	if rep.BytesFreed != 250 || rep.ObjectsRemoved != 7 {
		t.Fatalf("report %s", rep)
	}
	// A second run finds nothing new.
	del.keys = nil
	rep2, err := (&Sweeper{Cfg: c, Store: st, Objects: del}).Run(context.Background(), now)
	if err != nil || len(del.keys) != 0 || rep2.Expired[KindDocument] != 0 {
		t.Fatalf("second run: %v %v %s", err, del.keys, rep2)
	}
	// The checker reports expired media.
	ck := NewChecker(st, time.Minute)
	if !ck.Expired(context.Background(), KindDocument, 10) || ck.Expired(context.Background(), KindDocument, 11) {
		t.Fatal("checker")
	}
}

func TestSweeperCapAndErrors(t *testing.T) {
	st, del := world(), &fakeDeleter{}
	st.failHold = map[int64]bool{18: true}
	c := cfg()
	c.MaxPerRun = 1
	rep, err := (&Sweeper{Cfg: c, Store: st, Objects: del}).Run(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.expired) != 1 || !rep.CapReached || rep.Errors != 1 {
		t.Fatalf("report %s expired %v", rep, st.expired)
	}
}

func TestSweeperLiveNeedsDeleter(t *testing.T) {
	if _, err := (&Sweeper{Cfg: cfg(), Store: world()}).Run(context.Background(), now); err == nil {
		t.Fatal("want error without object storage")
	}
}

type countingStore struct {
	fakeStore
	n int
}

func (c *countingStore) ExpiredAt(ctx context.Context, k Kind, id int64) (int64, error) {
	c.n++
	return c.fakeStore.ExpiredAt(ctx, k, id)
}

func TestCheckerCaches(t *testing.T) {
	st := &countingStore{fakeStore: fakeStore{expired: map[int64]int64{5: 1}}}
	ck := NewChecker(st, time.Hour)
	for i := 0; i < 5; i++ {
		if !ck.Expired(context.Background(), KindDocument, 5) {
			t.Fatal("not expired")
		}
	}
	if st.n != 1 {
		t.Fatalf("lookups %d", st.n)
	}
	var nilChecker *Checker
	if nilChecker.Expired(context.Background(), KindDocument, 5) {
		t.Fatal("nil checker")
	}
}

func TestCheckerDoesNotCacheLive(t *testing.T) {
	st := &countingStore{fakeStore: fakeStore{expired: map[int64]int64{}}}
	ck := NewChecker(st, time.Hour)
	ctx := context.Background()
	if ck.Expired(ctx, KindDocument, 9) || ck.Expired(ctx, KindDocument, 9) {
		t.Fatal("live file looked expired")
	}
	if st.n != 2 {
		t.Fatalf("live lookups %d, want 2", st.n)
	}
	st.expired[9] = 50
	if !ck.Expired(ctx, KindDocument, 9) {
		t.Fatal("expired_at write was hidden")
	}
	n := st.n
	if !ck.Expired(ctx, KindDocument, 9) {
		t.Fatal("expired answer dropped")
	}
	if st.n != n {
		t.Fatalf("expired answer was not cached (%d -> %d)", n, st.n)
	}
	st.expired[9] = 0
	Forget(KindDocument, 9)
	if ck.Expired(ctx, KindDocument, 9) {
		t.Fatal("Forget left a stale expired answer")
	}
}
