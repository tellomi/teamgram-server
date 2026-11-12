package statusbook

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func TestSaveForgetAndOlderWrite(t *testing.T) {
	b := New(nil)
	ctx := context.Background()
	got, err := b.Load(ctx, 7)
	if err != nil || got.Pushed != 0 || got.HaveList {
		t.Fatalf("missing = %+v %v", got, err)
	}
	rec := Record{Pushed: 1_700_000_300, HaveList: true, Viewers: []Viewer{{ID: 2, Exact: true}}, Gen: got.Gen}
	if err = b.Save(ctx, 7, rec); err != nil {
		t.Fatal(err)
	}
	got, err = b.Load(ctx, 7)
	if err != nil || got.Pushed != rec.Pushed || len(got.Viewers) != 1 {
		t.Fatalf("saved = %+v %v", got, err)
	}
	older := got
	older.Pushed = rec.Pushed - 50
	older.Viewers = []Viewer{{ID: 9}}
	if err = b.Save(ctx, 7, older); err != nil {
		t.Fatal(err)
	}
	got, _ = b.Load(ctx, 7)
	if got.Pushed != rec.Pushed || got.Viewers[0].ID != 2 {
		t.Fatalf("older write replaced the mark: %+v", got)
	}
	b.Forget(ctx, 7)
	got, err = b.Load(ctx, 7)
	if err != nil || got.Pushed != 0 || got.HaveList || got.Gen == 0 {
		t.Fatalf("forgotten = %+v %v", got, err)
	}
}

func TestOversizedAudienceIsNotStored(t *testing.T) {
	b := New(nil)
	ctx := context.Background()
	viewers := make([]Viewer, MaxViewers+1)
	for i := range viewers {
		viewers[i] = Viewer{ID: int64(i + 1), Exact: true}
	}
	if err := b.Save(ctx, 4, Record{Pushed: 100, HaveList: true, Viewers: viewers}); err != nil {
		t.Fatal(err)
	}
	got, err := b.Load(ctx, 4)
	if err != nil || got.HaveList || len(got.Viewers) != 0 || got.Pushed != 100 {
		t.Fatalf("oversized list was kept: %+v %v", got, err)
	}
}

func TestInFlightSaveDoesNotRestoreForgottenList(t *testing.T) {
	assertInFlightSaveDropped(t, New(nil))
	mr := miniredis.RunT(t)
	store := kv.NewStore(cache.ClusterConf{{
		RedisConf: redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType},
		Weight:    100,
	}})
	assertInFlightSaveDropped(t, &Book{be: redisBE{kv: store}})
}

func assertInFlightSaveDropped(t *testing.T, b *Book) {
	t.Helper()
	ctx := context.Background()
	const userID int64 = 8
	if err := b.Save(ctx, userID, Record{
		Pushed: 300, HaveList: true, Gen: 0,
		Viewers: []Viewer{{ID: 1, Exact: true}},
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := b.Load(ctx, userID)
	if err != nil || !loaded.HaveList || loaded.Viewers[0].ID != 1 {
		t.Fatalf("seed %+v %v", loaded, err)
	}
	b.Forget(ctx, userID)
	stale := loaded
	stale.Pushed = 900
	if err = b.Save(ctx, userID, stale); err != nil {
		t.Fatal(err)
	}
	got, err := b.Load(ctx, userID)
	if err != nil || got.HaveList || got.Pushed != 0 || len(got.Viewers) != 0 {
		t.Fatalf("in-flight save wrote the old list back: %+v %v", got, err)
	}
	if got.Gen == loaded.Gen {
		t.Fatalf("forget did not bump the generation: %+v", got)
	}
	if err = b.Save(ctx, userID, Record{
		Pushed: 900, HaveList: true, Gen: got.Gen,
		Viewers: []Viewer{{ID: 2}},
	}); err != nil {
		t.Fatal(err)
	}
	got, err = b.Load(ctx, userID)
	if err != nil || !got.HaveList || got.Pushed != 900 || got.Viewers[0].ID != 2 {
		t.Fatalf("fresh generation was not stored: %+v %v", got, err)
	}
}

type errBackend struct{ err error }

func (e errBackend) load(context.Context, int64) (Record, error) { return Record{}, e.err }
func (e errBackend) save(context.Context, int64, Record) error   { return e.err }
func (e errBackend) forget(context.Context, []int64)             {}

func TestKVErrorsDoNotLookLikeAFreshSkip(t *testing.T) {
	readErr := errors.New("redis down")
	b := &Book{be: errBackend{err: readErr}}
	_, err := b.Load(context.Background(), 3)
	if !errors.Is(err, readErr) {
		t.Fatalf("load err %v", err)
	}
	if rec := OpenRecord(Record{Pushed: 10, HaveList: true, Gen: 4}, err); rec.Pushed != 0 || rec.HaveList || rec.Gen != 0 {
		t.Fatalf("read error was treated as a cached audience: %+v", rec)
	}
}

func TestStatusPrivacyForgetIDsIncludesBothContactDirections(t *testing.T) {
	const (
		self = int64(1)
		bob  = int64(2)
	)
	cases := []struct {
		name    string
		forward []int64
		reverse []int64
	}{
		{name: "nobody", reverse: []int64{bob}},
		{name: "contacts", forward: []int64{bob}},
		{name: "chat exception", reverse: []int64{bob}},
	}
	for _, tc := range cases {
		ids := StatusPrivacyForgetIDs(self, nil, tc.forward, tc.reverse)
		if !containsID(ids, self) || !containsID(ids, bob) {
			t.Fatalf("%s forget ids %v", tc.name, ids)
		}
	}
}

func containsID(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
