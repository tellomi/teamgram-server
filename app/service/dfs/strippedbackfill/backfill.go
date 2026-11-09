// Package strippedbackfill fills the inline profile-photo thumb (size "i")
// for photos that were stored before that thumb was written at upload time.
// It inserts one photo_sizes row and does not delete or replace objects.
package strippedbackfill

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
)

// CachedBytesMax is photo_sizes.cached_bytes. A longer thumb is not stored,
// because a truncated value would not decode.
const CachedBytesMax = 4096

// Size preference for the source JPEG. a/b/c/d are profile ladders.
// s/m/x/y/w/0 cover a photo that was stored on the message ladder.
var sourceSizes = []string{"a", "b", "c", "d", "s", "m", "x", "y", "w", "0"}

// Photo is one profile photo and the users and chats that show it.
type Photo struct {
	ID    int64
	Users []int64
	Chats []int64
}

// DB is the read path plus the single insert.
type DB interface {
	// Next returns profile photo ids strictly greater than after, ascending.
	Next(ctx context.Context, after int64, limit int) ([]int64, error)
	// Stripped reports whether size "i" exists and whether its bytes are empty.
	Stripped(ctx context.Context, photoID int64) (exists, filled bool, err error)
	// Paths maps a stored size type to its object key.
	Paths(ctx context.Context, photoID int64) (map[string]string, error)
	Owners(ctx context.Context, photoID int64) (users, chats []int64, err error)
	// SaveStripped inserts size "i", or fills cached_bytes when that row is empty.
	// A row that already has bytes is left unchanged.
	SaveStripped(ctx context.Context, photoID int64, raw []byte, cached string) error
}

// Blobs reads one stored JPEG. A missing object returns an error.
type Blobs interface {
	Read(ctx context.Context, path string) ([]byte, error)
}

// Cache drops the entries that still hold the photo without its thumb.
type Cache interface {
	Drop(ctx context.Context, keys ...string) error
}

// Encode builds a stripped JPEG from a normal JPEG.
type Encode func(jpeg []byte) ([]byte, error)

// Options controls one manual run.
type Options struct {
	After int64
	Batch int
	// Max is how many photos to consider. Zero means until Next is empty.
	Max    int
	DryRun bool
	// DropCache removes photo, user, and chat cache entries after a thumb
	// is stored (and on a later run, for photos that already have one).
	DropCache bool
	Encode    Encode
}

// Report is the outcome of one run. Last is the last photo id considered,
// including ones that were skipped, so the next run can pass it as After.
type Report struct {
	Scanned  int
	Filled   int
	Skipped  int
	Missing  int
	TooBig   int
	Failed   int
	CacheErr int
	Last     int64
}

func (r Report) String() string {
	return fmt.Sprintf("scanned=%d filled=%d skipped=%d missing=%d too_big=%d failed=%d cache_err=%d last=%d",
		r.Scanned, r.Filled, r.Skipped, r.Missing, r.TooBig, r.Failed, r.CacheErr, r.Last)
}

// Run walks profile photos after Options.After. It is safe to repeat:
// a photo that already has size "i" is not written again.
func Run(ctx context.Context, db DB, blobs Blobs, cache Cache, opt Options) (Report, error) {
	if db == nil || blobs == nil {
		return Report{}, fmt.Errorf("stripped backfill: db and blobs are required")
	}
	if opt.Encode == nil {
		return Report{}, fmt.Errorf("stripped backfill: encoder is required")
	}
	if opt.Batch <= 0 {
		opt.Batch = 50
	}
	var rep Report
	after := opt.After
	for {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		n := opt.Batch
		if opt.Max > 0 {
			left := opt.Max - rep.Scanned
			if left <= 0 {
				return rep, nil
			}
			if left < n {
				n = left
			}
		}
		ids, err := db.Next(ctx, after, n)
		if err != nil {
			return rep, err
		}
		if len(ids) == 0 {
			return rep, nil
		}
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return rep, err
			}
			after = id
			rep.Last = id
			rep.Scanned++
			if err := one(ctx, db, blobs, cache, opt, id, &rep); err != nil {
				rep.Failed++
				return rep, err
			}
		}
	}
}

func one(ctx context.Context, db DB, blobs Blobs, cache Cache, opt Options, id int64, rep *Report) error {
	_, filled, err := db.Stripped(ctx, id)
	if err != nil {
		return err
	}
	if filled {
		rep.Skipped++
		if opt.DropCache && !opt.DryRun {
			return drop(ctx, db, cache, id, rep)
		}
		return nil
	}
	paths, err := db.Paths(ctx, id)
	if err != nil {
		return err
	}
	raw, ok := thumbFrom(ctx, blobs, opt.Encode, id, paths)
	if !ok {
		rep.Missing++
		return nil
	}
	cached := base64.RawStdEncoding.EncodeToString(raw)
	if len(cached) > CachedBytesMax {
		rep.TooBig++
		return nil
	}
	if opt.DryRun {
		rep.Filled++
		return nil
	}
	if err := db.SaveStripped(ctx, id, raw, cached); err != nil {
		return err
	}
	rep.Filled++
	if opt.DropCache {
		return drop(ctx, db, cache, id, rep)
	}
	return nil
}

func thumbFrom(ctx context.Context, blobs Blobs, encode Encode, id int64, paths map[string]string) ([]byte, bool) {
	seen := map[string]struct{}{}
	for _, sz := range sourceSizes {
		path := ""
		if paths != nil {
			path = paths[sz]
		}
		if path == "" {
			if _, listed := paths[sz]; paths != nil && !listed {
				continue
			}
			path = fmt.Sprintf("%s/%d.dat", sz, id)
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		jpeg, err := blobs.Read(ctx, path)
		if err != nil || len(jpeg) == 0 {
			continue
		}
		raw, err := encode(jpeg)
		if err != nil || len(raw) == 0 {
			continue
		}
		return raw, true
	}
	return nil, false
}

func drop(ctx context.Context, db DB, cache Cache, id int64, rep *Report) error {
	if cache == nil {
		rep.CacheErr++
		return nil
	}
	users, chats, err := db.Owners(ctx, id)
	if err != nil {
		return err
	}
	keys := CacheKeys(id, users, chats)
	if err := cache.Drop(ctx, keys...); err != nil {
		rep.CacheErr++
		return nil
	}
	return nil
}

// CacheKeys are the redis entries that embed this photo.
// photo# is the media cache. user_data.2# and chat# embed that photo
// for up to seven days, so the new thumb is invisible until they are dropped.
func CacheKeys(photoID int64, users, chats []int64) []string {
	keys := []string{fmt.Sprintf("photo#%d", photoID)}
	seenU := map[int64]struct{}{}
	for _, id := range users {
		if id == 0 {
			continue
		}
		if _, ok := seenU[id]; ok {
			continue
		}
		seenU[id] = struct{}{}
		keys = append(keys, fmt.Sprintf("user_data.2#%d", id))
	}
	seenC := map[int64]struct{}{}
	sort.Slice(chats, func(i, j int) bool { return chats[i] < chats[j] })
	for _, id := range chats {
		if id == 0 {
			continue
		}
		if _, ok := seenC[id]; ok {
			continue
		}
		seenC[id] = struct{}{}
		keys = append(keys, fmt.Sprintf("chat#%d", id))
	}
	return keys
}
