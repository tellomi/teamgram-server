package minio_util

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
)

const (
	defaultChunkMemBytes  int64 = 64 << 20
	defaultChunkDiskBytes int64 = 512 << 20
	// diskChunkMin is the smallest chunk worth a file. Smaller reads stay
	// in the memory LRU only.
	diskChunkMin = 32 << 10
)

// chunkCache is a two-tier LRU of upload.getFile ranges. Documents already
// checked the short-lived part cache and then read object storage directly;
// photo sizes never checked either. Both now share this cache. Objects are
// immutable for a key, so a hit does not revalidate.
type chunkCache struct {
	memCap  int64
	diskCap int64
	dir     string

	mu       sync.Mutex
	mem      *list.List // front is newest
	memBytes int64
	memIdx   map[string]*list.Element

	disk      *list.List
	diskBytes int64
	diskIdx   map[string]*list.Element

	inflight map[string]*chunkCall

	hits   atomic.Int64
	misses atomic.Int64
}

type memEntry struct {
	key  string
	data []byte
}

type diskEntry struct {
	key  string
	name string
	size int64
}

type chunkCall struct {
	done chan struct{}
	data []byte
	err  error
}

func newChunkCacheFromEnv() *chunkCache {
	mem := envBytes("TELLOMI_DFS_CHUNK_CACHE_MEM", defaultChunkMemBytes)
	disk := envBytes("TELLOMI_DFS_CHUNK_CACHE_DISK", defaultChunkDiskBytes)
	dir := os.Getenv("TELLOMI_DFS_CHUNK_CACHE_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "tellomi-dfs-chunk-cache")
	}
	return newChunkCache(mem, disk, dir)
}

func newChunkCache(memCap, diskCap int64, dir string) *chunkCache {
	c := &chunkCache{
		memCap:   memCap,
		diskCap:  diskCap,
		dir:      dir,
		mem:      list.New(),
		memIdx:   map[string]*list.Element{},
		disk:     list.New(),
		diskIdx:  map[string]*list.Element{},
		inflight: map[string]*chunkCall{},
	}
	if diskCap > 0 && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			c.diskCap = 0
		}
	}
	if c.memCap <= 0 && c.diskCap <= 0 {
		return nil
	}
	return c
}

func envBytes(name string, def int64) int64 {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func chunkKey(class, path string, offset int64, limit int32) string {
	return class + "\x00" + path + "\x00" + strconv.FormatInt(offset, 10) + "\x00" + strconv.FormatInt(int64(limit), 10)
}

// getOrLoad returns a cached range or calls load once for this key.
// The returned slice is owned by the caller.
func (c *chunkCache) getOrLoad(ctx context.Context, key string, load func(context.Context) ([]byte, error)) ([]byte, error) {
	if c == nil {
		return load(ctx)
	}
	if data, ok := c.get(key); ok {
		c.hits.Add(1)
		return data, nil
	}
	call, leader := c.begin(key)
	if !leader {
		select {
		case <-call.done:
			if call.err != nil {
				return nil, call.err
			}
			c.hits.Add(1)
			return append([]byte(nil), call.data...), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	data, err := load(ctx)
	if err == nil && len(data) > 0 {
		c.put(key, data)
	}
	c.finish(key, call, data, err)
	c.misses.Add(1)
	return data, err
}

func (c *chunkCache) begin(key string) (*chunkCall, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if call, ok := c.inflight[key]; ok {
		return call, false
	}
	call := &chunkCall{done: make(chan struct{})}
	c.inflight[key] = call
	return call, true
}

func (c *chunkCache) finish(key string, call *chunkCall, data []byte, err error) {
	call.data = append([]byte(nil), data...)
	call.err = err
	c.mu.Lock()
	if c.inflight[key] == call {
		delete(c.inflight, key)
	}
	c.mu.Unlock()
	close(call.done)
}

func (c *chunkCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.memIdx[key]; ok {
		c.mem.MoveToFront(el)
		ent := el.Value.(*memEntry)
		return append([]byte(nil), ent.data...), true
	}
	el, ok := c.diskIdx[key]
	if !ok {
		return nil, false
	}
	c.disk.MoveToFront(el)
	ent := el.Value.(*diskEntry)
	data, err := os.ReadFile(ent.name)
	if err != nil || int64(len(data)) != ent.size {
		c.disk.Remove(el)
		delete(c.diskIdx, key)
		c.diskBytes -= ent.size
		if ent.name != "" {
			_ = os.Remove(ent.name)
		}
		return nil, false
	}
	c.addMemLocked(key, data)
	return append([]byte(nil), data...), true
}

func (c *chunkCache) put(key string, data []byte) {
	if len(data) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addMemLocked(key, data)
	if c.diskCap > 0 && int64(len(data)) >= diskChunkMin && int64(len(data)) <= c.diskCap {
		c.addDiskLocked(key, data)
	}
}

func (c *chunkCache) addMemLocked(key string, data []byte) {
	if c.memCap <= 0 || int64(len(data)) > c.memCap {
		return
	}
	if el, ok := c.memIdx[key]; ok {
		c.mem.Remove(el)
		c.memBytes -= int64(len(el.Value.(*memEntry).data))
		delete(c.memIdx, key)
	}
	buf := append([]byte(nil), data...)
	el := c.mem.PushFront(&memEntry{key: key, data: buf})
	c.memIdx[key] = el
	c.memBytes += int64(len(buf))
	for c.memBytes > c.memCap && c.mem.Len() > 0 {
		back := c.mem.Back()
		ent := back.Value.(*memEntry)
		c.mem.Remove(back)
		delete(c.memIdx, ent.key)
		c.memBytes -= int64(len(ent.data))
	}
}

func (c *chunkCache) addDiskLocked(key string, data []byte) {
	if el, ok := c.diskIdx[key]; ok {
		c.disk.MoveToFront(el)
		return
	}
	sum := sha256.Sum256([]byte(key))
	name := filepath.Join(c.dir, hex.EncodeToString(sum[:]))
	tmp := name + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, name); err != nil {
		_ = os.Remove(tmp)
		return
	}
	el := c.disk.PushFront(&diskEntry{key: key, name: name, size: int64(len(data))})
	c.diskIdx[key] = el
	c.diskBytes += int64(len(data))
	for c.diskBytes > c.diskCap && c.disk.Len() > 0 {
		back := c.disk.Back()
		ent := back.Value.(*diskEntry)
		c.disk.Remove(back)
		delete(c.diskIdx, ent.key)
		c.diskBytes -= ent.size
		_ = os.Remove(ent.name)
	}
}

func cacheableReadClass(class string) bool {
	switch class {
	case ClassPhotos, ClassDocuments:
		return true
	default:
		return false
	}
}
