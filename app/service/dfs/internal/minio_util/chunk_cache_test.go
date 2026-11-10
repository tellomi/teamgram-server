package minio_util

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestChunkCacheHitsAndEvicts(t *testing.T) {
	c := newChunkCache(30, 0, t.TempDir())
	loads := atomic.Int32{}
	first := bytes.Repeat([]byte("p"), 20)
	key := chunkKey(ClassPhotos, "x/1.dat", 0, 128)
	got, err := c.getOrLoad(context.Background(), key, func(context.Context) ([]byte, error) {
		loads.Add(1)
		return first, nil
	})
	if err != nil || !bytes.Equal(got, first) {
		t.Fatalf("load %d bytes %v", len(got), err)
	}
	got, err = c.getOrLoad(context.Background(), key, func(context.Context) ([]byte, error) {
		loads.Add(1)
		return first, nil
	})
	if err != nil || !bytes.Equal(got, first) || loads.Load() != 1 {
		t.Fatalf("second load count %d err %v", loads.Load(), err)
	}
	// The cap holds one 20-byte chunk. The next one evicts the first.
	if _, err = c.getOrLoad(context.Background(), chunkKey(ClassDocuments, "2.dat", 0, 128), func(context.Context) ([]byte, error) {
		return bytes.Repeat([]byte("d"), 20), nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.getOrLoad(context.Background(), key, func(context.Context) ([]byte, error) {
		loads.Add(1)
		return first, nil
	}); err != nil {
		t.Fatal(err)
	}
	if loads.Load() != 2 {
		t.Fatalf("evicted chunk was not loaded again, loads=%d", loads.Load())
	}
}

func TestChunkCacheSingleflight(t *testing.T) {
	c := newChunkCache(1<<20, 0, t.TempDir())
	var loads atomic.Int32
	start := make(chan struct{})
	load := func(context.Context) ([]byte, error) {
		loads.Add(1)
		<-start
		return []byte("once"), nil
	}
	key := chunkKey(ClassPhotos, "m/9.dat", 0, 512)
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := c.getOrLoad(context.Background(), key, load)
			if err != nil || string(got) != "once" {
				errCh <- err
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("object storage reads %d, want 1", loads.Load())
	}
}

func TestChunkCacheDiskSurvivesMemoryEviction(t *testing.T) {
	dir := t.TempDir()
	// Memory holds one chunk. Disk keeps it after the memory tier drops it.
	const n = 36 << 10
	c := newChunkCache(40<<10, 1<<20, dir)
	big := bytes.Repeat([]byte("p"), n)
	key := chunkKey(ClassPhotos, "y/3.dat", 0, 1024)
	if _, err := c.getOrLoad(context.Background(), key, func(context.Context) ([]byte, error) {
		return big, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.getOrLoad(context.Background(), chunkKey(ClassPhotos, "s/3.dat", 0, 64), func(context.Context) ([]byte, error) {
		return bytes.Repeat([]byte("s"), n), nil
	}); err != nil {
		t.Fatal(err)
	}
	loads := 0
	got, err := c.getOrLoad(context.Background(), key, func(context.Context) ([]byte, error) {
		loads++
		return []byte("should-not-load"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if loads != 0 || !bytes.Equal(got, big) {
		t.Fatalf("disk hit loads=%d got %d bytes", loads, len(got))
	}
}

func TestEmptyAndErrorsAreNotCached(t *testing.T) {
	c := newChunkCache(1<<20, 0, t.TempDir())
	key := chunkKey(ClassDocuments, "4.dat", 0, 32)
	n := 0
	for i := 0; i < 2; i++ {
		_, err := c.getOrLoad(context.Background(), key, func(context.Context) ([]byte, error) {
			n++
			return nil, context.DeadlineExceeded
		})
		if err == nil {
			t.Fatal("expected error")
		}
	}
	if n != 2 {
		t.Fatalf("error was cached, loads=%d", n)
	}
	emptyKey := chunkKey(ClassDocuments, "empty.dat", 0, 32)
	n = 0
	for i := 0; i < 2; i++ {
		got, err := c.getOrLoad(context.Background(), emptyKey, func(context.Context) ([]byte, error) {
			n++
			return []byte{}, nil
		})
		if err != nil || len(got) != 0 {
			t.Fatalf("empty read %d %v", len(got), err)
		}
	}
	if n != 2 {
		t.Fatalf("empty success was cached, loads=%d", n)
	}
}

func TestNegativeCacheExpires(t *testing.T) {
	c := newChunkCache(1<<20, 0, t.TempDir())
	now := time.Now()
	c.now = func() time.Time { return now }
	key := chunkKey(ClassStickers, "9.dat", 0, 32)
	n := 0
	load := func(context.Context) ([]byte, error) {
		n++
		return nil, ErrNotFound
	}
	if _, err := c.getOrLoad(context.Background(), key, load); err == nil {
		t.Fatal("expected miss")
	}
	if _, err := c.getOrLoad(context.Background(), key, load); err == nil {
		t.Fatal("expected cached miss")
	}
	if n != 1 {
		t.Fatalf("second miss hit storage, loads=%d", n)
	}
	now = now.Add(negativeCacheTTL + time.Second)
	if _, err := c.getOrLoad(context.Background(), key, load); err == nil {
		t.Fatal("expected miss after ttl")
	}
	if n != 2 {
		t.Fatalf("expired miss was not loaded, loads=%d", n)
	}
}

func TestStickerClassIsCacheable(t *testing.T) {
	if !cacheableReadClass(ClassStickers) || !cacheableReadClass(ClassEmoji) || !cacheableReadClass(ClassAvatars) {
		t.Fatal("sticker, emoji, and avatar reads are not cached")
	}
	c := newChunkCache(1<<20, 0, t.TempDir())
	body := []byte("sticker-bytes")
	key := chunkKey(ClassStickers, "42.dat", 0, 128)
	n := 0
	load := func(context.Context) ([]byte, error) {
		n++
		return body, nil
	}
	got, err := c.getOrLoad(context.Background(), key, load)
	if err != nil || string(got) != string(body) {
		t.Fatal(err)
	}
	got, err = c.getOrLoad(context.Background(), key, load)
	if err != nil || string(got) != string(body) || n != 1 {
		t.Fatalf("second sticker read loads=%d err=%v", n, err)
	}
}

func osNotFound() error { return ErrNotFound }
