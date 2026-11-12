package statusbook

import (
	"context"
	"errors"
	"testing"
)

func TestSaveForgetAndOlderWrite(t *testing.T) {
	b := New(nil)
	ctx := context.Background()
	got, err := b.Load(ctx, 7)
	if err != nil || got.Pushed != 0 || got.HaveList {
		t.Fatalf("missing = %+v %v", got, err)
	}
	rec := Record{Pushed: 1_700_000_300, HaveList: true, Viewers: []Viewer{{ID: 2, Exact: true}}}
	if err = b.Save(ctx, 7, rec); err != nil {
		t.Fatal(err)
	}
	got, err = b.Load(ctx, 7)
	if err != nil || got.Pushed != rec.Pushed || len(got.Viewers) != 1 {
		t.Fatalf("saved = %+v %v", got, err)
	}
	if err = b.Save(ctx, 7, Record{Pushed: rec.Pushed - 50, HaveList: true, Viewers: []Viewer{{ID: 9}}}); err != nil {
		t.Fatal(err)
	}
	got, _ = b.Load(ctx, 7)
	if got.Pushed != rec.Pushed || got.Viewers[0].ID != 2 {
		t.Fatalf("older write replaced the mark: %+v", got)
	}
	b.Forget(ctx, 7)
	got, err = b.Load(ctx, 7)
	if err != nil || got.Pushed != 0 || got.HaveList {
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

type failKV struct {
	getErr error
	setErr error
}

func (f failKV) GetCtx(context.Context, string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	return "", nil
}

func (f failKV) SetexCtx(context.Context, string, string, int) error { return f.setErr }

func (f failKV) DelCtx(context.Context, ...string) (int, error) { return 0, nil }

func TestKVErrorsDoNotLookLikeAFreshSkip(t *testing.T) {
	readErr := errors.New("redis down")
	b := &Book{kv: failKV{getErr: readErr}}
	_, err := b.Load(context.Background(), 3)
	if !errors.Is(err, readErr) {
		t.Fatalf("load err %v", err)
	}
	if rec := OpenRecord(Record{Pushed: 10, HaveList: true}, err); rec.Pushed != 0 || rec.HaveList {
		t.Fatalf("read error was treated as a cached audience: %+v", rec)
	}
	writeErr := errors.New("redis write")
	b.kv = failKV{setErr: writeErr}
	if err = b.Save(context.Background(), 3, Record{Pushed: 20, HaveList: true}); !errors.Is(err, writeErr) {
		t.Fatalf("save err %v", err)
	}
}
