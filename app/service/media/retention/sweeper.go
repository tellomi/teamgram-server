package retention

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/app/service/mediablob"
)

// Deleter removes one object. objectstore.Client implements it.
type Deleter interface {
	Delete(ctx context.Context, class, path string) error
}

// Sweeper applies the retention policy.
type Sweeper struct {
	Cfg     Config
	Store   Store
	Objects Deleter
	Logf    func(format string, args ...any)
}

// Report summarises one run.
type Report struct {
	DryRun         bool
	Started        time.Time
	Cutoff         time.Time
	Scanned        map[Kind]int
	Reasons        map[Reason]int
	Expired        map[Kind]int
	BytesFreed     int64
	ObjectsRemoved int
	Errors         int
	CapReached     bool
	// EncryptedOld counts secret-chat files past the cutoff. They are never
	// removed: secret chats are memory-only, so their participants are unknown.
	EncryptedOld int64
	Samples      []string
}

func (r Report) String() string {
	var b strings.Builder
	mode := "LIVE"
	if r.DryRun {
		mode = "DRY_RUN"
	}
	fmt.Fprintf(&b, "media retention %s: cutoff=%s scanned documents=%d photos=%d; ", mode, r.Cutoff.UTC().Format(time.RFC3339), r.Scanned[KindDocument], r.Scanned[KindPhoto])
	keys := make([]string, 0, len(r.Reasons))
	for k := range r.Reasons {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%d ", k, r.Reasons[Reason(k)])
	}
	verb := "expired"
	if r.DryRun {
		verb = "would_expire"
	}
	fmt.Fprintf(&b, "; %s documents=%d photos=%d bytes=%d objects_removed=%d errors=%d encrypted_old_skipped=%d cap_reached=%v",
		verb, r.Expired[KindDocument], r.Expired[KindPhoto], r.BytesFreed, r.ObjectsRemoved, r.Errors, r.EncryptedOld, r.CapReached)
	return b.String()
}

const maxSamples = 50

func (s *Sweeper) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
}

// Run makes one pass over documents and photos older than the cutoff.
func (s *Sweeper) Run(ctx context.Context, now time.Time) (Report, error) {
	cfg := s.Cfg
	cfg.Normalize()
	cutoff := now.Add(-cfg.Retention())
	rep := Report{
		DryRun:  cfg.DryRun,
		Started: now,
		Cutoff:  cutoff,
		Scanned: map[Kind]int{},
		Reasons: map[Reason]int{},
		Expired: map[Kind]int{},
	}
	if !cfg.DryRun && s.Objects == nil {
		return rep, fmt.Errorf("media retention: object storage is not configured")
	}
	if n, err := s.Store.CountOld(ctx, KindEncrypted, cutoff.Unix()); err == nil {
		rep.EncryptedOld = n
	} else {
		s.logf("media retention: count encrypted files: %v", err)
	}
	var channels map[Kind]map[int64][]int64
	expired := 0
	for _, kind := range []Kind{KindDocument, KindPhoto} {
		var after int64
		for {
			if err := ctx.Err(); err != nil {
				return rep, err
			}
			batch, err := s.Store.ListCandidates(ctx, kind, cutoff.Unix(), after, cfg.Batch)
			if err != nil {
				return rep, err
			}
			for _, m := range batch {
				after = m.ID
				rep.Scanned[kind]++
				// Exempt and young items skip the holder lookups.
				reason := Decide(m, nil, now, cfg)
				if reason == ReasonUnreferenced {
					if channels == nil {
						if channels, err = s.Store.ChannelMedia(ctx); err != nil {
							return rep, err
						}
					}
					var holders []Subscription
					holders, err = s.holders(ctx, m, channels)
					if err != nil {
						rep.Errors++
						s.logf("media retention: holders of %s %d: %v", m.Kind, m.ID, err)
						continue
					}
					reason = Decide(m, holders, now, cfg)
				}
				if reason == ReasonExpire && cfg.MaxPerRun > 0 && expired >= cfg.MaxPerRun {
					rep.CapReached = true
					continue
				}
				rep.Reasons[reason]++
				if reason != ReasonExpire {
					continue
				}
				expired++
				if err := s.expire(ctx, m, now, cfg.DryRun, &rep); err != nil {
					rep.Errors++
					s.logf("media retention: expire %s %d: %v", m.Kind, m.ID, err)
					continue
				}
				rep.Expired[kind]++
				rep.BytesFreed += m.Size
				if len(rep.Samples) < maxSamples {
					rep.Samples = append(rep.Samples, fmt.Sprintf("%s %d uploaded=%s size=%d mime=%q",
						m.Kind, m.ID, time.Unix(m.UploadedAt, 0).UTC().Format("2006-01-02"), m.Size, m.MimeType))
				}
			}
			if len(batch) < cfg.Batch {
				break
			}
		}
	}
	return rep, nil
}

// holders returns the subscription state of everyone who can still see m:
// owners of live message boxes, current basic-group members and current
// channel members.
func (s *Sweeper) holders(ctx context.Context, m Media, channels map[Kind]map[int64][]int64) ([]Subscription, error) {
	users := map[int64]bool{}
	boxes, err := s.Store.MessageHolders(ctx, m.Kind, m.ID)
	if err != nil {
		return nil, err
	}
	for _, h := range boxes {
		users[h.UserID] = true
		if h.PeerType == peerChat {
			ids, err := s.Store.ChatMembers(ctx, h.PeerID)
			if err != nil {
				return nil, err
			}
			for _, id := range ids {
				users[id] = true
			}
		}
	}
	for _, ch := range channels[m.Kind][m.ID] {
		ids, err := s.Store.ChannelMembers(ctx, ch)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			users[id] = true
		}
		if len(ids) == 0 {
			// A channel with no members left still references the media;
			// keep it as referenced so it is not treated as orphaned.
			users[0] = true
		}
	}
	if len(users) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(users))
	for id := range users {
		if id != 0 {
			ids = append(ids, id)
		}
	}
	subs, err := s.Store.Subscriptions(ctx, ids)
	if err != nil {
		return nil, err
	}
	// Users missing from the users table still count as free holders.
	seen := map[int64]bool{}
	for _, sub := range subs {
		seen[sub.UserID] = true
	}
	for id := range users {
		if !seen[id] {
			subs = append(subs, Subscription{UserID: id})
		}
	}
	return subs, nil
}

func (s *Sweeper) expire(ctx context.Context, m Media, now time.Time, dryRun bool, rep *Report) error {
	objs, err := s.retentionObjects(ctx, m, dryRun)
	if err != nil {
		return err
	}
	if dryRun {
		s.logf("media retention DRY_RUN: would expire %s %d (%d objects, %d bytes)", m.Kind, m.ID, len(objs), m.Size)
		return nil
	}
	for _, o := range objs {
		if err := s.Objects.Delete(ctx, o[0], o[1]); err != nil {
			return err
		}
		rep.ObjectsRemoved++
	}
	if err := s.Store.MarkExpired(ctx, m.Kind, m.ID, now.Unix()); err != nil {
		return err
	}
	s.logf("media retention: expired %s %d (%d objects, %d bytes)", m.Kind, m.ID, len(objs), m.Size)
	return nil
}

// retentionObjects is the object list expire would remove. A blob that
// still has another document or photo keeps its object. Dry runs do not
// change the reference count.
func (s *Sweeper) retentionObjects(ctx context.Context, m Media, dryRun bool) ([][2]string, error) {
	objs := Objects(m)
	db := s.blobDB()
	if db == nil || (m.Kind != KindDocument && m.Kind != KindPhoto) {
		return objs, nil
	}
	blobs := mediablob.New(db)
	kind := mediablob.KindDocument
	if m.Kind == KindPhoto {
		kind = mediablob.KindPhoto
	}
	if !dryRun {
		rel, err := blobs.Release(ctx, kind, m.ID)
		if err != nil {
			return nil, err
		}
		if rel.Found && !rel.Remove {
			objs = nil
		} else if rel.Found && rel.Remove && rel.Path != "" {
			objs = releasedObjects(m, rel)
		}
	}
	return dropSharedObjects(ctx, blobs, m, objs)
}

func releasedObjects(m Media, rel mediablob.ReleaseResult) [][2]string {
	if m.Kind == KindDocument {
		if rel.Class == "" {
			rel.Class = "documents"
		}
		return [][2]string{{rel.Class, rel.Path}}
	}
	id := originFromPhotoPath(rel.Path)
	if id == 0 {
		id = m.ID
	}
	return Objects(Media{Kind: KindPhoto, ID: id})
}

func originFromPhotoPath(path string) int64 {
	rest := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		rest = path[i+1:]
	}
	rest = strings.TrimSuffix(rest, ".dat")
	id, _ := strconv.ParseInt(rest, 10, 64)
	return id
}

// dropSharedObjects refuses to delete an object key that another row still
// references. That covers a sweeper retry after the reference row was
// already removed.
func dropSharedObjects(ctx context.Context, blobs *mediablob.Store, m Media, objs [][2]string) ([][2]string, error) {
	if len(objs) == 0 || blobs == nil {
		return objs, nil
	}
	var class, path string
	switch m.Kind {
	case KindDocument:
		class, path = "documents", itoa(m.ID)+".dat"
	case KindPhoto:
		class, path = "photos", "0/"+itoa(m.ID)+".dat"
	default:
		return objs, nil
	}
	inUse, err := blobs.PathInUse(ctx, class, path)
	if err != nil {
		return nil, err
	}
	if inUse {
		return nil, nil
	}
	return objs, nil
}

func (s *Sweeper) blobDB() *sql.DB {
	st, ok := s.Store.(*SQLStore)
	if !ok || st == nil {
		return nil
	}
	return st.DB
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// NextRun returns the next time at hour:00 local after now.
func NextRun(now time.Time, hour int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// Schedule runs the sweeper once a day at cfg.RunHour until ctx ends.
func (s *Sweeper) Schedule(ctx context.Context) {
	for {
		next := NextRun(time.Now(), s.Cfg.RunHour)
		s.logf("media retention: next run at %s (dry_run=%v, days=%d, grace_days=%d)", next.Format(time.RFC3339), s.Cfg.DryRun, s.Cfg.Days, s.Cfg.GraceDays)
		t := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		runCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
		rep, err := s.Run(runCtx, time.Now())
		cancel()
		if err != nil {
			s.logf("media retention: run failed: %v (%s)", err, rep)
			continue
		}
		s.logf("%s", rep)
	}
}
