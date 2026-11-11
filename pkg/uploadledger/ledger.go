// Package uploadledger splits a chat upload into network bytes, new object
// bytes, logical references, and an unfinished reservation. The subject is
// always the uploader. A target chat is not charged.
package uploadledger

import (
	"errors"
	"sync"
	"time"
)

// ErrLimited is a real budget failure. Callers must not store the part or
// report success.
var ErrLimited = errors.New("upload budget exceeded")

// Limits are rolling 24h caps for one uploader. Zero means that meter is off.
type Limits struct {
	Network int64
	Entity  int64
	Hold    int64
	Refs    int64
	Window  time.Duration
}

// DefaultLimits sits above a normal album and a long video, and below an
// open loop of new objects.
func DefaultLimits() Limits {
	return Limits{
		Network: 8 << 30,
		Entity:  8 << 30,
		Hold:    1 << 30,
		Refs:    100000,
		Window:  24 * time.Hour,
	}
}

type entry struct {
	at   time.Time
	n    int64
	kind string
}

// Ledger is the process ledger. Same part bytes are idempotent. A different
// length is a new charge.
type Ledger struct {
	mu    sync.Mutex
	lim   Limits
	now   func() time.Time
	seen  map[string]int64
	items map[int64][]entry
}

func New(lim Limits) *Ledger {
	if lim.Window <= 0 {
		lim.Window = 24 * time.Hour
	}
	return &Ledger{
		lim:   lim,
		now:   time.Now,
		seen:  map[string]int64{},
		items: map[int64][]entry{},
	}
}

var (
	sharedMu sync.RWMutex
	shared   *Ledger
)

// Use installs the process ledger. Nil disables charging.
func Use(l *Ledger) {
	sharedMu.Lock()
	shared = l
	sharedMu.Unlock()
}

// Shared is the process ledger, or nil.
func Shared() *Ledger {
	sharedMu.RLock()
	defer sharedMu.RUnlock()
	return shared
}

// Book is one uploader's meters inside the window.
type Book struct {
	Network int64
	Entity  int64
	Refs    int64
	Hold    int64
}

func (l *Ledger) Book(user int64) Book {
	if l == nil {
		return Book{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(user, l.now())
	return l.sumLocked(user)
}

// ChargePart records network bytes for one part. The same user, file, part
// and length is a retry and does not charge again. fresh is false for that
// retry. A different length charges again.
func (l *Ledger) ChargePart(user, fileID int64, part int, nbytes int64) (fresh bool, err error) {
	if l == nil || user == 0 || nbytes <= 0 {
		return false, nil
	}
	key := partKey(user, fileID, part, nbytes)
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.pruneLocked(user, now)
	if _, ok := l.seen[key]; ok {
		return false, nil
	}
	if l.lim.Network > 0 && l.sumLocked(user).Network+nbytes > l.lim.Network {
		return false, ErrLimited
	}
	l.seen[key] = nbytes
	l.items[user] = append(l.items[user], entry{at: now, n: nbytes, kind: "network"})
	return true, nil
}

// RefundPart gives back a charge that was not stored. A retry that was not
// fresh must not be refunded.
func (l *Ledger) RefundPart(user, fileID int64, part int, nbytes int64) {
	if l == nil || user == 0 || nbytes <= 0 {
		return
	}
	key := partKey(user, fileID, part, nbytes)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen[key] != nbytes {
		return
	}
	delete(l.seen, key)
	items := l.items[user]
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].kind == "network" && items[i].n == nbytes {
			l.items[user] = append(items[:i], items[i+1:]...)
			return
		}
	}
}

// Reserve holds unfinished bytes. Cancel returns only this hold.
func (l *Ledger) Reserve(user int64, nbytes int64) error {
	return l.add(user, nbytes, "hold", l.lim.Hold)
}

// Cancel returns an unused reservation. It does not return network bytes
// already stored or entity bytes already committed.
func (l *Ledger) Cancel(user int64, nbytes int64) {
	l.remove(user, nbytes, "hold")
}

// CommitNew turns a reservation into a new object. The hold is released and
// the entity meter increases. A dedup hit must use CommitReuse instead.
func (l *Ledger) CommitNew(user int64, nbytes int64) error {
	if err := l.add(user, nbytes, "entity", l.lim.Entity); err != nil {
		return err
	}
	l.remove(user, nbytes, "hold")
	return nil
}

// CommitReuse records a logical reference. The uploader is charged one ref.
// Entity bytes stay with the original upload. peer is ignored so a target
// chat cannot be charged.
func (l *Ledger) CommitReuse(user, peer int64) error {
	_ = peer
	return l.add(user, 1, "ref", l.lim.Refs)
}

func (l *Ledger) add(user, n int64, kind string, limit int64) error {
	if l == nil || user == 0 || n <= 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.pruneLocked(user, now)
	cur := l.kindLocked(user, kind)
	if limit > 0 && cur+n > limit {
		return ErrLimited
	}
	l.items[user] = append(l.items[user], entry{at: now, n: n, kind: kind})
	return nil
}

func (l *Ledger) remove(user, n int64, kind string) {
	if l == nil || user == 0 || n <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	items := l.items[user]
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].kind == kind && items[i].n == n {
			l.items[user] = append(items[:i], items[i+1:]...)
			return
		}
	}
}

func (l *Ledger) pruneLocked(user int64, now time.Time) {
	items := l.items[user]
	cut := now.Add(-l.lim.Window)
	dst := items[:0]
	for _, it := range items {
		if !it.at.Before(cut) {
			dst = append(dst, it)
		}
	}
	l.items[user] = dst
}

func (l *Ledger) sumLocked(user int64) Book {
	var b Book
	for _, it := range l.items[user] {
		switch it.kind {
		case "network":
			b.Network += it.n
		case "entity":
			b.Entity += it.n
		case "ref":
			b.Refs += it.n
		case "hold":
			b.Hold += it.n
		}
	}
	return b
}

func (l *Ledger) kindLocked(user int64, kind string) int64 {
	var n int64
	for _, it := range l.items[user] {
		if it.kind == kind {
			n += it.n
		}
	}
	return n
}

func partKey(user, file int64, part int, n int64) string {
	return itoa(user) + ":" + itoa(file) + ":" + itoa(int64(part)) + ":" + itoa(n)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
