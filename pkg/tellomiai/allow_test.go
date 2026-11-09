package tellomiai

import (
	"context"
	"errors"
	"testing"
	"time"
)

type scriptDir struct {
	calls [][]string
	ids   map[string]int64
	err   error
}

func (d *scriptDir) Users(_ context.Context, names []string) (map[string]int64, error) {
	d.calls = append(d.calls, append([]string(nil), names...))
	if d.err != nil {
		return nil, d.err
	}
	out := map[string]int64{}
	for _, name := range names {
		if id, ok := d.ids[name]; ok {
			out[name] = id
		}
	}
	return out, nil
}

func TestAllowListNamesIDsAndStar(t *testing.T) {
	ctx := context.Background()
	dir := &scriptDir{ids: map[string]int64{"erel": 9, "pikaqiu": 11}}
	list := ParseAllow("Erel, @PikaQiu, erel", "", dir)

	ok, err := list.Allowed(ctx, 9)
	if err != nil || !ok {
		t.Fatalf("erel allowed=%v err=%v", ok, err)
	}
	ok, err = list.Allowed(ctx, 8)
	if err != nil || ok {
		t.Fatalf("stranger allowed=%v err=%v", ok, err)
	}
	if len(dir.calls) != 1 {
		t.Fatalf("lookups %v", dir.calls)
	}
	got := dir.calls[0]
	if len(got) != 2 || got[0] != "erel" || got[1] != "pikaqiu" {
		t.Fatalf("names %v", got)
	}

	ids := ParseAllow("", "9, 10, no", nil)
	ok, err = ids.Allowed(ctx, 10)
	if err != nil || !ok {
		t.Fatalf("id allowed=%v err=%v", ok, err)
	}
	ok, err = ids.Allowed(ctx, 8)
	if err != nil || ok {
		t.Fatalf("other id allowed=%v err=%v", ok, err)
	}

	boom := &scriptDir{err: errors.New("lookup should not run")}
	all := ParseAllow("*", "", boom)
	ok, err = all.Allowed(ctx, 12345)
	if err != nil || !ok || len(boom.calls) != 0 {
		t.Fatalf("star allowed=%v err=%v calls=%d", ok, err, len(boom.calls))
	}
}

func TestAllowFromEnvDefaultsToTheThreeNames(t *testing.T) {
	t.Setenv(EnvAllowUsernames, "")
	t.Setenv(EnvAllowUserIDs, "42")
	dir := &scriptDir{ids: map[string]int64{
		"erel": 1, "lfnuebfci": 2, "pikaqiu": 3,
	}}
	list := AllowFromEnv(dir)
	ok, err := list.Allowed(context.Background(), 2)
	if err != nil || !ok {
		t.Fatalf("default name allowed=%v err=%v", ok, err)
	}
	ok, err = list.Allowed(context.Background(), 42)
	if err != nil || !ok {
		t.Fatalf("configured id allowed=%v err=%v", ok, err)
	}
	if len(dir.calls) != 1 {
		t.Fatalf("lookups %d", len(dir.calls))
	}
	want := []string{"erel", "lfnuebfci", "pikaqiu"}
	got := dir.calls[0]
	if len(got) != len(want) {
		t.Fatalf("names %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names %v", got)
		}
	}
}

func TestAllowCacheAndLookupError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)
	dir := &scriptDir{ids: map[string]int64{"erel": 9}}
	list := ParseAllow("erel", "", dir)
	list.now = func() time.Time { return now }

	if _, err := list.Allowed(ctx, 9); err != nil {
		t.Fatal(err)
	}
	if _, err := list.Allowed(ctx, 9); err != nil {
		t.Fatal(err)
	}
	if len(dir.calls) != 1 {
		t.Fatalf("cache missed: %d", len(dir.calls))
	}
	now = now.Add(AllowCacheFor + time.Second)
	if _, err := list.Allowed(ctx, 9); err != nil {
		t.Fatal(err)
	}
	if len(dir.calls) != 2 {
		t.Fatalf("cache stuck: %d", len(dir.calls))
	}

	dir.err = errors.New("down")
	now = now.Add(AllowCacheFor + time.Second)
	if _, err := list.Allowed(ctx, 9); err == nil {
		t.Fatal("lookup failure was ignored")
	}
	if _, err := list.Allowed(ctx, 9); err == nil {
		t.Fatal("error was not cached")
	}
	if len(dir.calls) != 3 {
		t.Fatalf("error cache missed: %d", len(dir.calls))
	}
	now = now.Add(allowErrorCacheFor + time.Second)
	dir.err = nil
	ok, err := list.Allowed(ctx, 9)
	if err != nil || !ok || len(dir.calls) != 4 {
		t.Fatalf("recovered allowed=%v err=%v calls=%d", ok, err, len(dir.calls))
	}
}
