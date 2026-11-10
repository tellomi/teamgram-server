package mediapurge

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/teamgram/teamgram-server/app/service/mediablob"
)

// Deleter removes one object. A missing object is success.
type Deleter interface {
	Delete(ctx context.Context, class, path string) error
}

// Result is one worker pass. It is safe to log: no message text, no hashes,
// no credentials.
type Result struct {
	DryRun   bool
	Due      int
	Held     int
	Released int
	Objects  int
	Bytes    int64
	Shared   int
	Legacy   int
	Errors   int
	Samples  []string
}

func (r Result) String() string {
	mode := "LIVE"
	if r.DryRun {
		mode = "DRY_RUN"
	}
	return fmt.Sprintf("media purge %s: due=%d held=%d released=%d objects=%d bytes=%d shared_blob=%d kept_legacy=%d errors=%d",
		mode, r.Due, r.Held, r.Released, r.Objects, r.Bytes, r.Shared, r.Legacy, r.Errors)
}

// Run processes due queue rows. Dry-run only logs. ignoreGrace processes
// rows whose grace has not elapsed; the steady-state worker leaves it false.
func (s *Store) Run(ctx context.Context, del Deleter, now time.Time, batch int, dryRun, ignoreGrace bool) (Result, error) {
	var rep Result
	rep.DryRun = dryRun
	if s == nil || s.db == nil {
		return rep, nil
	}
	if err := s.Ensure(ctx); err != nil {
		return rep, err
	}
	items, err := s.Due(ctx, now, batch, ignoreGrace)
	if err != nil {
		return rep, err
	}
	rep.Due = len(items)
	cache := &holdCache{}
	for _, it := range items {
		if err = ctx.Err(); err != nil {
			return rep, err
		}
		one, err := s.one(ctx, del, now, Media{Kind: it.Kind, ID: it.ID}, dryRun, cache)
		if err != nil {
			rep.Errors++
			_ = s.Fail(ctx, Media{Kind: it.Kind, ID: it.ID}, now.Add(time.Minute), err.Error())
			if len(rep.Samples) < 8 {
				rep.Samples = append(rep.Samples, fmt.Sprintf("error %s %d", it.Kind, it.ID))
			}
			continue
		}
		rep.Held += one.Held
		rep.Released += one.Released
		rep.Objects += one.Objects
		rep.Bytes += one.Bytes
		rep.Shared += one.Shared
		rep.Legacy += one.Legacy
		if len(rep.Samples) < 8 && (one.Released > 0 || one.Objects > 0) {
			rep.Samples = append(rep.Samples, fmt.Sprintf("%s %d objects=%d", it.Kind, it.ID, one.Objects))
		}
	}
	return rep, nil
}

func (s *Store) one(ctx context.Context, del Deleter, now time.Time, m Media, dryRun bool, cache *holdCache) (Result, error) {
	var rep Result
	hold, err := s.FullHold(ctx, m, cache)
	if err != nil {
		return rep, err
	}
	if hold != HoldNone {
		rep.Held = 1
		if !dryRun {
			if err = s.Ack(ctx, m); err != nil {
				return rep, err
			}
		}
		return rep, nil
	}
	objs, err := s.Objects(ctx, m)
	if err != nil {
		return rep, err
	}
	blocked, why, err := s.blockedOutside(ctx, m, objs)
	if err != nil {
		return rep, err
	}
	if blocked {
		// Fail closed. Do not release the blob ref and do not delete the key.
		rep.Legacy = 1
		logger.Printf("media purge: skip %s %d held outside media_blob_refs (%s)", m.Kind, m.ID, why)
		if len(rep.Samples) < 8 {
			rep.Samples = append(rep.Samples, "legacy "+why)
		}
		if !dryRun {
			return s.finish(ctx, m, rep)
		}
		return rep, nil
	}
	kind := blobKind(m.Kind)
	blobs := mediablob.New(s.db)
	if dryRun {
		rel, err := blobs.Peek(ctx, kind, m.ID)
		if err != nil && !missing(err) {
			return rep, err
		}
		if rel.Found && !rel.Remove {
			rep.Shared = 1
			return rep, nil
		}
		rep.Released = 1
		for _, o := range objs {
			held, err := s.pathHeld(ctx, o.Class, o.Path, m)
			if err != nil {
				return rep, err
			}
			if held {
				continue
			}
			rep.Objects++
			rep.Bytes += o.Bytes
		}
		return rep, nil
	}
	rel, err := blobs.Release(ctx, kind, m.ID)
	if err != nil && !missing(err) {
		return rep, err
	}
	if rel.Found && !rel.Remove {
		// The row keeps file_path after this ref is gone. Mark it expired so
		// upload.getFile stops and a later pass does not treat that leftover
		// path as a legacy holder of the shared object.
		if err = s.markExpired(ctx, m, now); err != nil {
			return rep, err
		}
		rep.Shared = 1
		return s.finish(ctx, m, rep)
	}
	rep.Released = 1
	if del == nil && len(objs) > 0 {
		return rep, fmt.Errorf("object storage is not configured")
	}
	// Expire this id before the path check. Its file_path remains after the
	// blob ref is released, and that leftover must not look like a legacy holder.
	if err = s.markExpired(ctx, m, now); err != nil {
		return rep, err
	}
	heldAny := false
	for _, o := range objs {
		held, err := s.pathHeld(ctx, o.Class, o.Path, m)
		if err != nil {
			return rep, err
		}
		if held {
			heldAny = true
			_ = blobs.AckGC(ctx, o.Class, o.Path)
			continue
		}
		if err = del.Delete(ctx, o.Class, o.Path); err != nil {
			return rep, err
		}
		_ = blobs.AckGC(ctx, o.Class, o.Path)
		rep.Objects++
		rep.Bytes += o.Bytes
	}
	// A sibling still uses the key, so the object stays. This id does not:
	// its messages are gone, and upload.getFile refuses expired_at.
	if heldAny && rep.Objects == 0 {
		rep.Shared = 1
		if err = s.markExpired(ctx, m, now); err != nil {
			return rep, err
		}
		return s.finish(ctx, m, rep)
	}
	if err = s.markExpired(ctx, m, now); err != nil {
		return rep, err
	}
	return s.finish(ctx, m, rep)
}

func (s *Store) finish(ctx context.Context, m Media, rep Result) (Result, error) {
	if err := s.Ack(ctx, m); err != nil {
		return rep, err
	}
	return rep, nil
}

func (s *Store) markExpired(ctx context.Context, m Media, now time.Time) error {
	table, col := "documents", "document_id"
	if m.Kind == KindPhoto {
		table, col = "photos", "photo_id"
	}
	_, err := s.db.ExecContext(ctx, `UPDATE `+table+` SET expired_at=? WHERE `+col+`=? AND expired_at=0`, now.Unix(), m.ID)
	if missing(err) || missingColumn(err) {
		return nil
	}
	return err
}

// Loop runs until ctx ends. It logs one line per pass.
func (s *Store) Loop(ctx context.Context, del Deleter, cfg Config) {
	if s == nil {
		return
	}
	tick := time.NewTicker(cfg.Interval)
	defer tick.Stop()
	s.pass(ctx, del, cfg)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.pass(ctx, del, cfg)
		}
	}
}

func (s *Store) pass(ctx context.Context, del Deleter, cfg Config) {
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	rep, err := s.Run(runCtx, del, time.Now(), cfg.Batch, cfg.DryRun, false)
	if err != nil {
		logger.Printf("media purge: run failed: %v (%s)", err, rep)
		return
	}
	if rep.Due > 0 {
		logger.Printf("%s", rep)
	}
}

// OpenDB is the connection the worker and the command use.
func OpenDB(dsn string) (*sql.DB, error) {
	st, err := Open(dsn)
	if err != nil || st == nil {
		return nil, err
	}
	return st.db, nil
}
