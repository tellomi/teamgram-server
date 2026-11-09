package strippedbackfill

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type memDB struct {
	order    []int64
	filled   map[int64]string
	emptyRow map[int64]bool
	paths    map[int64]map[string]string
	users    map[int64][]int64
	chats    map[int64][]int64
	saved    []int64
}

func (m *memDB) Next(_ context.Context, after int64, limit int) ([]int64, error) {
	var out []int64
	for _, id := range m.order {
		if id <= after {
			continue
		}
		out = append(out, id)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (m *memDB) Stripped(_ context.Context, id int64) (bool, bool, error) {
	if s, ok := m.filled[id]; ok && s != "" {
		return true, true, nil
	}
	if m.emptyRow[id] {
		return true, false, nil
	}
	return false, false, nil
}

func (m *memDB) Paths(_ context.Context, id int64) (map[string]string, error) {
	if m.paths == nil {
		return map[string]string{}, nil
	}
	return m.paths[id], nil
}

func (m *memDB) Owners(_ context.Context, id int64) ([]int64, []int64, error) {
	return m.users[id], m.chats[id], nil
}

func (m *memDB) SaveStripped(_ context.Context, id int64, raw []byte, cached string) error {
	if m.filled[id] != "" {
		return nil
	}
	if m.filled == nil {
		m.filled = map[int64]string{}
	}
	m.filled[id] = cached
	m.saved = append(m.saved, id)
	if len(raw) == 0 || cached == "" {
		return errors.New("empty thumb")
	}
	return nil
}

type memBlobs struct {
	files map[string][]byte
}

func (b memBlobs) Read(_ context.Context, path string) ([]byte, error) {
	v, ok := b.files[path]
	if !ok {
		return nil, errors.New("missing")
	}
	return v, nil
}

type memCache struct {
	keys []string
	err  error
}

func (c *memCache) Drop(_ context.Context, keys ...string) error {
	c.keys = append(c.keys, keys...)
	return c.err
}

func TestRunFillsMissingAndSkipsFilled(t *testing.T) {
	db := &memDB{
		order:  []int64{2, 5, 9},
		filled: map[int64]string{2: "already"},
		paths: map[int64]map[string]string{
			5: {"c": "c/5.dat"},
			9: {"a": "shared/a.dat"},
		},
		users: map[int64][]int64{5: {10, 10}, 9: {11}},
		chats: map[int64][]int64{5: {3}},
	}
	blobs := memBlobs{files: map[string][]byte{
		"c/5.dat":      []byte("jpeg-5"),
		"shared/a.dat": []byte("jpeg-9"),
	}}
	cache := &memCache{}
	rep, err := Run(context.Background(), db, blobs, cache, Options{
		Batch:     2,
		DropCache: true,
		Encode: func(jpeg []byte) ([]byte, error) {
			return append([]byte("i:"), jpeg...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 3 || rep.Skipped != 1 || rep.Filled != 2 || rep.Missing != 0 || rep.Last != 9 {
		t.Fatalf("%s", rep)
	}
	if len(db.saved) != 2 || db.saved[0] != 5 || db.saved[1] != 9 {
		t.Fatalf("saved %v", db.saved)
	}
	if !strings.Contains(strings.Join(cache.keys, ","), "photo#5") ||
		!strings.Contains(strings.Join(cache.keys, ","), "user_data.2#10") ||
		!strings.Contains(strings.Join(cache.keys, ","), "chat#3") ||
		!strings.Contains(strings.Join(cache.keys, ","), "photo#2") {
		t.Fatalf("cache %v", cache.keys)
	}
	// A second run writes nothing and still refreshes the cache.
	cache.keys = nil
	rep, err = Run(context.Background(), db, blobs, cache, Options{
		Batch: 10, DropCache: true,
		Encode: func([]byte) ([]byte, error) { return []byte{9}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Filled != 0 || rep.Skipped != 3 || len(db.saved) != 2 {
		t.Fatalf("second %s saved %v", rep, db.saved)
	}
	if len(cache.keys) == 0 {
		t.Fatal("re-run should still drop cache")
	}
}

func TestDryRunDoesNotWrite(t *testing.T) {
	db := &memDB{
		order: []int64{4},
		paths: map[int64]map[string]string{4: {"a": "a/4.dat"}},
	}
	cache := &memCache{}
	rep, err := Run(context.Background(), db, memBlobs{files: map[string][]byte{"a/4.dat": {1}}}, cache, Options{
		DryRun: true, DropCache: true, Batch: 10,
		Encode: func([]byte) ([]byte, error) { return []byte{7, 7}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Filled != 1 || len(db.saved) != 0 || len(cache.keys) != 0 {
		t.Fatalf("%s saved %v cache %v", rep, db.saved, cache.keys)
	}
}

func TestResumeAndMissingJPEG(t *testing.T) {
	db := &memDB{
		order: []int64{1, 2, 8},
		paths: map[int64]map[string]string{8: {"b": "b/8.dat"}},
	}
	rep, err := Run(context.Background(), db, memBlobs{files: map[string][]byte{"b/8.dat": {1}}}, nil, Options{
		After: 1, Batch: 10,
		Encode: func([]byte) ([]byte, error) { return []byte{1}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 2 || rep.Missing != 1 || rep.Filled != 1 || rep.Last != 8 {
		t.Fatalf("%s", rep)
	}
	if len(db.saved) != 1 || db.saved[0] != 8 {
		t.Fatalf("saved %v", db.saved)
	}
}

func TestTooBigIsNotStored(t *testing.T) {
	db := &memDB{
		order: []int64{3},
		paths: map[int64]map[string]string{3: {"a": "a/3.dat"}},
	}
	rep, err := Run(context.Background(), db, memBlobs{files: map[string][]byte{"a/3.dat": {1}}}, nil, Options{
		Batch: 5,
		Encode: func([]byte) ([]byte, error) {
			return make([]byte, CachedBytesMax), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.TooBig != 1 || rep.Filled != 0 || len(db.saved) != 0 {
		t.Fatalf("%s", rep)
	}
}

func TestMergeIDsKeepsTheNextBatch(t *testing.T) {
	got := mergeIDs([][]int64{{1, 5}, {2, 3}, {5, 9}}, 3)
	if strings.Join(ints(got), ",") != "1,2,3" {
		t.Fatalf("%v", got)
	}
}

func ints(ids []int64) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = fmt.Sprint(id)
	}
	return out
}

func TestCacheKeys(t *testing.T) {
	keys := CacheKeys(9, []int64{1, 1, 0}, []int64{4, 2, 4})
	got := strings.Join(keys, " ")
	if got != "photo#9 user_data.2#1 chat#2 chat#4" {
		t.Fatalf("%s", got)
	}
}
