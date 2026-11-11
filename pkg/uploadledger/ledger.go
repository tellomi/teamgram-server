// Package uploadledger splits a chat upload into network bytes, new object
// bytes, logical references, and an unfinished reservation. The subject is
// always the uploader. A target chat is not charged.
package uploadledger

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"
)

const (
	ModeOff     = "off"
	ModeLog     = "log"
	ModeEnforce = "enforce"

	maxUsers = 8192
	maxHolds = 1024
)

// ErrLimited is a real budget failure. Callers must not store the part or
// report success.
var ErrLimited = errors.New("upload budget exceeded")

// Limited is a budget failure with the remaining window.
type Limited struct {
	Wait time.Duration
}

func (e *Limited) Error() string { return ErrLimited.Error() }
func (e *Limited) Unwrap() error { return ErrLimited }

// WaitOf is the suggested FLOOD_WAIT. A non-limit error waits one second.
func WaitOf(err error) int32 {
	var lim *Limited
	if errors.As(err, &lim) && lim.Wait > 0 {
		sec := int32(lim.Wait / time.Second)
		if lim.Wait%time.Second != 0 {
			sec++
		}
		if sec < 1 {
			return 1
		}
		if sec > 86400 {
			return 86400
		}
		return sec
	}
	return 1
}

// Limits are caps for one uploader inside Window. Zero means that meter is off.
type Limits struct {
	Network int64
	Entity  int64
	Hold    int64
	Refs    int64
	Window  time.Duration
}

// DefaultLimits sits above a normal album and a long video.
func DefaultLimits() Limits {
	return Limits{
		Network: 8 << 30,
		Entity:  8 << 30,
		Hold:    1 << 30,
		Refs:    100000,
		Window:  24 * time.Hour,
	}
}

type meterEvent struct {
	at time.Time
	n  int64
}

type counter struct {
	n      int64
	start  time.Time
	events []meterEvent
}

type hold struct {
	n    int64
	at   time.Time
	file int64
}

const maxMeterEvents = 32

// Ledger meters one process, or delegates to Redis when constructed with
// NewRedis. Network bytes are every received byte. A retry and a same-length
// replacement both count. Holds are cancelled only by the id Reserve returned.
type Ledger struct {
	mu       sync.Mutex
	lim      Limits
	mode     string
	now      func() time.Time
	net      map[int64]*counter
	entity   map[int64]*counter
	refs     map[int64]*counter
	holdSum  map[int64]*counter
	holds    map[int64]map[int64]hold
	seen     map[int64]time.Time
	nextHold int64
	redis    redisAdder
}

func New(lim Limits) *Ledger {
	return newLedger(lim, ModeEnforce)
}

func newLedger(lim Limits, mode string) *Ledger {
	if lim.Window <= 0 {
		lim.Window = 24 * time.Hour
	}
	if mode == "" {
		mode = ModeLog
	}
	return &Ledger{
		lim:     lim,
		mode:    mode,
		now:     time.Now,
		net:     map[int64]*counter{},
		entity:  map[int64]*counter{},
		refs:    map[int64]*counter{},
		holdSum: map[int64]*counter{},
		holds:   map[int64]map[int64]hold{},
		seen:    map[int64]time.Time{},
	}
}

// InstallFromEnv installs the process ledger. The default mode is log, so a
// deploy does not start enforcing the caps. Enforce requires
// TELLOMI_UPLOAD_LEDGER_REDIS so every files and dfs process shares the
// counters. There is no SQL migration.
func InstallFromEnv() {
	mode := os.Getenv("TELLOMI_UPLOAD_LEDGER_MODE")
	if mode == "" {
		mode = ModeLog
	}
	lim := DefaultLimits()
	addr := os.Getenv("TELLOMI_UPLOAD_LEDGER_REDIS")
	if addr != "" {
		if led, err := openRedis(addr, lim, mode); err == nil {
			Use(led)
			return
		}
	}
	if mode == ModeEnforce && addr == "" {
		mode = ModeLog
	}
	Use(newLedger(lim, mode))
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

type userKey struct{}

// WithUser returns a context the dfs upload path reads.
func WithUser(ctx context.Context, user int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, userKey{}, user)
}

// UserFrom is the uploader stashed by WithUser.
func UserFrom(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	id, _ := ctx.Value(userKey{}).(int64)
	return id
}

// NoteClaim records a finished object. A reuse is one logical reference and
// does not add entity bytes. peer is ignored.
func NoteClaim(user, fileID int64, reused bool, size int64) error {
	led := Shared()
	if led == nil || user == 0 {
		return nil
	}
	if err := led.allowClaim(reused, user, size); err != nil {
		return err
	}
	led.ReleaseFile(user, fileID)
	return nil
}

// AllowClaim reports whether this object may be stored. It does not release
// holds and does not write anything.
func AllowClaim(user int64, reused bool, size int64) error {
	led := Shared()
	if led == nil || user == 0 {
		return nil
	}
	return led.allowClaim(reused, user, size)
}

func (l *Ledger) allowClaim(reused bool, user, size int64) error {
	if reused {
		return l.CommitReuse(user, 0)
	}
	return l.CommitNew(user, size)
}

// Users is how many subjects currently occupy the local ledger.
func (l *Ledger) Users() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.seen)
}

// ReleaseFile drops holds for one upload id. Other files stay reserved.
func (l *Ledger) ReleaseFile(user, fileID int64) {
	if l == nil || user == 0 {
		return
	}
	if l.redis != nil {
		l.redis.releaseFile(user, fileID)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, h := range l.holds[user] {
		if h.file == fileID {
			delete(l.holds[user], id)
		}
	}
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
	if l.redis != nil {
		return l.redis.book(user, l.lim.Window)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	return Book{
		Network: l.value(l.net, user, now),
		Entity:  l.value(l.entity, user, now),
		Refs:    l.value(l.refs, user, now),
		Hold:    l.holdValue(user, now),
	}
}

// ChargeNetwork adds bytes the process has already received. It does not
// look at file id, part, or length for an exemption. The same bytes sent
// again count again.
func (l *Ledger) ChargeNetwork(user, n int64) error {
	return l.add("net", l.net, user, n, l.lim.Network)
}

// Reserve holds unfinished bytes and returns the id Cancel must use.
func (l *Ledger) Reserve(user, fileID, n int64) (int64, error) {
	if l == nil || user == 0 || n <= 0 || l.mode == ModeOff {
		return 0, nil
	}
	if l.redis != nil {
		return l.redis.reserve(user, fileID, n, l.lim.Hold, l.lim.Window, l.mode == ModeEnforce)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	user = l.slot(user, now)
	if l.mode == ModeEnforce && l.lim.Hold > 0 && l.holdValue(user, now)+n > l.lim.Hold {
		return 0, &Limited{Wait: l.lim.Window}
	}
	bucket := l.holds[user]
	if bucket == nil {
		bucket = map[int64]hold{}
		l.holds[user] = bucket
	}
	if len(bucket) >= maxHolds {
		if l.mode == ModeEnforce {
			return 0, &Limited{Wait: l.lim.Window}
		}
		return 0, nil
	}
	l.nextHold++
	id := l.nextHold
	bucket[id] = hold{n: n, at: now, file: fileID}
	return id, nil
}

// Cancel drops one unused reservation. It does not change network or entity
// bytes, and it does not touch a different id with the same length.
func (l *Ledger) Cancel(id int64) {
	if l == nil || id == 0 {
		return
	}
	if l.redis != nil {
		l.redis.cancel(id)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, bucket := range l.holds {
		if _, ok := bucket[id]; !ok {
			continue
		}
		delete(bucket, id)
		return
	}
}

// CommitNew records new object bytes. It does not refund network bytes.
func (l *Ledger) CommitNew(user, n int64) error {
	return l.add("entity", l.entity, user, n, l.lim.Entity)
}

// CommitReuse records one logical reference for the uploader. peer is ignored
// so a target chat cannot be charged. Entity bytes stay with the original.
func (l *Ledger) CommitReuse(user, peer int64) error {
	_ = peer
	return l.add("refs", l.refs, user, 1, l.lim.Refs)
}

func (l *Ledger) add(kind string, bucket map[int64]*counter, user, n, limit int64) error {
	if l == nil || user == 0 || n <= 0 || l.mode == ModeOff {
		return nil
	}
	if l.redis != nil {
		return l.redis.add(kind, user, n, limit, l.lim.Window, l.mode == ModeEnforce)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	user = l.slot(user, now)
	if err := l.allow(bucket, user, n, limit, now); err != nil {
		return err
	}
	l.bump(bucket, user, n, now)
	return nil
}

func (l *Ledger) slot(user int64, now time.Time) int64 {
	for id, at := range l.seen {
		if id == overflowUser {
			continue
		}
		if now.Sub(at) >= l.lim.Window {
			delete(l.seen, id)
			delete(l.net, id)
			delete(l.entity, id)
			delete(l.refs, id)
			delete(l.holdSum, id)
			delete(l.holds, id)
		}
	}
	if _, ok := l.seen[user]; ok {
		l.seen[user] = now
		return user
	}
	if len(l.seen) >= maxUsers {
		l.seen[overflowUser] = now
		return overflowUser
	}
	l.seen[user] = now
	return user
}

const overflowUser int64 = -1

func (l *Ledger) allow(bucket map[int64]*counter, user, n, limit int64, now time.Time) error {
	if l.mode != ModeEnforce || limit <= 0 {
		return nil
	}
	cur := l.live(bucket, user, now)
	if cur+n > limit {
		return &Limited{Wait: l.remaining(bucket, user, now)}
	}
	return nil
}

func (l *Ledger) bump(bucket map[int64]*counter, user, n int64, now time.Time) {
	c := l.liveCounter(bucket, user, now)
	c.events = append(c.events, meterEvent{at: now, n: n})
	for len(c.events) > maxMeterEvents {
		c.events[0].n += c.events[1].n
		c.events = append(c.events[:1], c.events[2:]...)
	}
	c.n += n
}

func (l *Ledger) value(bucket map[int64]*counter, user int64, now time.Time) int64 {
	return l.live(bucket, user, now)
}

func (l *Ledger) live(bucket map[int64]*counter, user int64, now time.Time) int64 {
	c := l.liveCounter(bucket, user, now)
	if c == nil {
		return 0
	}
	return c.n
}

func (l *Ledger) liveCounter(bucket map[int64]*counter, user int64, now time.Time) *counter {
	c := bucket[user]
	if c == nil {
		c = &counter{start: now}
		bucket[user] = c
	}
	dst := c.events[:0]
	var sum int64
	start := now
	for _, ev := range c.events {
		if now.Sub(ev.at) >= l.lim.Window {
			continue
		}
		dst = append(dst, ev)
		sum += ev.n
		if ev.at.Before(start) {
			start = ev.at
		}
	}
	c.events = dst
	c.n = sum
	c.start = start
	return c
}

func (l *Ledger) remaining(bucket map[int64]*counter, user int64, now time.Time) time.Duration {
	c := bucket[user]
	if c == nil {
		return l.lim.Window
	}
	left := l.lim.Window - now.Sub(c.start)
	if left < time.Second {
		return time.Second
	}
	return left
}

func (l *Ledger) holdValue(user int64, now time.Time) int64 {
	var sum int64
	for _, h := range l.holds[user] {
		if now.Sub(h.at) < l.lim.Window {
			sum += h.n
		}
	}
	return sum
}
