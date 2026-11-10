package msgexpiry

import (
	"sync"
	"time"
)

// Kind is the chat boundary a record belongs to. A sweep of one kind does
// not delete another.
type Kind string

const (
	KindPrivate  Kind = "private"
	KindChannel  Kind = "channel"
	KindSecret   Kind = "secret"
	KindViewOnce Kind = "view_once"
)

// Record is one synthetic or newly observed message. It is not a row loaded
// from a production table.
type Record struct {
	Kind    Kind
	Scope   int64
	ID      int32
	Date    int32
	TTL     int32
	FileID  int64
	Users   []int64
	Opened  bool
	Deleted bool
}

type key struct {
	kind  Kind
	scope int64
	id    int32
}

// Ledger is the in-process expiry table. It does not open MySQL and does not
// read backup objects. A restart forgets it; rows already in the database
// stay until a caller that holds them passes them to Sweep.
type Ledger struct {
	mu    sync.Mutex
	items map[key]*Record
}

func NewLedger() *Ledger {
	return &Ledger{items: map[key]*Record{}}
}

var (
	defaultOnce sync.Once
	defaultLed  *Ledger
)

// Default is the process ledger. It starts empty.
func Default() *Ledger {
	defaultOnce.Do(func() { defaultLed = NewLedger() })
	return defaultLed
}

// Register stores a message the feature is allowed to expire. A repeat of
// the same key keeps the first ttl and does not clear Deleted.
func (l *Ledger) Register(r Record) {
	if l == nil || r.Kind == "" || r.ID == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	k := key{r.Kind, r.Scope, r.ID}
	if prev, ok := l.items[k]; ok {
		if prev.FileID == 0 {
			prev.FileID = r.FileID
		}
		return
	}
	cp := r
	cp.Users = append([]int64(nil), r.Users...)
	l.items[k] = &cp
	if cp.Kind == KindSecret {
		saveSecret(cp)
	}
}

// Open marks a view-once record as seen. Other kinds are left alone.
func (l *Ledger) Open(scope int64, id int32) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if rec, ok := l.items[key{KindViewOnce, scope, id}]; ok {
		rec.Opened = true
	}
}

// Readable is false when the switch is on and the record is expired, already
// deleted, or a view-once that has been opened. The switch off means readable.
func (l *Ledger) Readable(kind Kind, scope int64, id int32, now int32) bool {
	if !Enabled() || l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	rec, ok := l.items[key{kind, scope, id}]
	if !ok {
		return true
	}
	return !blocked(rec, now)
}

// BlocksDownload reports whether this file id must not be served. Thumbnails
// are not registered. The switch off never blocks.
func (l *Ledger) BlocksDownload(fileID int64, now int32) bool {
	if !Enabled() || l == nil || fileID == 0 {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, rec := range l.items {
		if rec.FileID == fileID && blocked(rec, now) {
			return true
		}
	}
	return false
}

func blocked(rec *Record, now int32) bool {
	if rec == nil || rec.Deleted {
		return rec != nil && rec.Deleted
	}
	if rec.Kind == KindViewOnce && rec.Opened {
		return true
	}
	return Expired(rec.Date, rec.TTL, now)
}

// Sweep marks expired or opened view-once records deleted and returns the
// ones deleted by this call, once per record. A second call returns nothing.
// It does not run unless the sweep switch is on, and it only walks this
// ledger. Backups and unrelated tables are not touched.
func (l *Ledger) Sweep(now int32) []Record {
	if !SweepEnabled() || l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Record
	for _, rec := range l.items {
		if rec.Deleted || !blocked(rec, now) {
			continue
		}
		rec.Deleted = true
		cp := *rec
		cp.Users = append([]int64(nil), rec.Users...)
		out = append(out, cp)
		if cp.Kind == KindSecret {
			markSecretSwept(cp)
		}
	}
	return out
}

// UsersOf lists the accounts that must drop a deleted record. Duplicates
// are removed so a reconnect does not notify the same device twice for one sweep.
func UsersOf(recs []Record) []int64 {
	seen := map[int64]struct{}{}
	var out []int64
	for _, rec := range recs {
		for _, id := range rec.Users {
			if id == 0 {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

// Guard starts the in-process sweep loop only when the sweep switch is on
// at process start. Tests call Sweep directly. The loop never reads MySQL.
func Guard(l *Ledger) {
	if !SweepEnabled() || l == nil {
		return
	}
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for now := range tick.C {
			l.Sweep(int32(now.Unix()))
		}
	}()
}
