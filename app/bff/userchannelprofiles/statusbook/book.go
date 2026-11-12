// Package statusbook remembers who last received a user's online status and
// the expires that was sent. A later heartbeat reuses that list to slide
// userStatusOnline.expires and does not read contacts, privacy, or blocks
// again. Contact, privacy, and block changes bump a generation and drop the
// list. A save from a heartbeat that read the old generation does not write
// it back. Redis is used when KV is configured. Otherwise the mark stays in
// this process, under the same generation rule.
package statusbook

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/teamgram/marmota/pkg/stores/kv"
	gokv "github.com/zeromicro/go-zero/core/stores/kv"
)

const (
	// TTL bounds how long a missed invalidation can keep a list. A successful
	// push refreshes it. The generation counter is not deleted with the list.
	TTL = 600
	// MaxViewers bounds one user's cached audience. A larger list is not
	// stored; the next heartbeat loads it again instead of keeping it.
	MaxViewers = 512
)

const keyPrefix = "status_aud_v2:"

// saveScript writes the audience only when the generation still matches the
// one the caller read and the new expires is strictly later. Otherwise it
// leaves the key alone.
const saveScript = `
local key = KEYS[1]
local expect = tostring(ARGV[1])
local newPushed = tonumber(ARGV[2])
local body = ARGV[3]
local ttl = tonumber(ARGV[4])
local gen = redis.call('HGET', key, 'gen')
if gen == false or gen == nil then gen = '0' end
if tostring(gen) ~= expect then return 0 end
local cur = redis.call('HGET', key, 'pushed')
local curPushed = 0
if cur and cur ~= false then curPushed = tonumber(cur) or 0 end
if newPushed <= curPushed then return 0 end
redis.call('HSET', key, 'pushed', tostring(newPushed), 'body', body)
redis.call('EXPIRE', key, ttl)
return 1
`

// forgetScript bumps the generation and drops the list. The generation stays.
const forgetScript = `
local key = KEYS[1]
redis.call('HINCRBY', key, 'gen', 1)
redis.call('HDEL', key, 'pushed', 'body')
redis.call('PERSIST', key)
return 1
`

// Viewer is one cached recipient. Exact is the precise online or offline
// status. Otherwise the recipient gets the coarse bucket, and ByMe is set
// when that bucket exists because they hid their own last seen.
type Viewer struct {
	ID    int64 `json:"id"`
	Exact bool  `json:"exact,omitempty"`
	ByMe  bool  `json:"by_me,omitempty"`
}

// Record is the last successful fanout. HaveList is false when the list was
// never stored, was forgotten, or was too large to keep. Gen is the
// generation observed by Load; Save must send that same value back.
type Record struct {
	Pushed   int64    `json:"pushed,omitempty"`
	Viewers  []Viewer `json:"viewers,omitempty"`
	HaveList bool     `json:"have_list,omitempty"`
	Gen      int64    `json:"gen,omitempty"`
}

type body struct {
	Viewers  []Viewer `json:"viewers,omitempty"`
	HaveList bool     `json:"have_list,omitempty"`
}

type backend interface {
	load(ctx context.Context, userID int64) (Record, error)
	save(ctx context.Context, userID int64, rec Record) error
	forget(ctx context.Context, userIDs []int64)
}

// Book is the shared last-audience mark.
type Book struct {
	be backend
}

// New uses Redis when conf is set and an in-process map otherwise.
func New(conf gokv.KvConf) *Book {
	if len(conf) == 0 {
		return &Book{be: newMem()}
	}
	return &Book{be: redisBE{kv: kv.NewStore(conf)}}
}

var (
	sharedMu sync.Mutex
	shared   = New(nil)
)

// Shared is the book account.updateStatus and the contact/privacy handlers share.
func Shared() *Book {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	return shared
}

// Install replaces the process-wide book. Startup calls it after opening
// Redis. Tests call it so cases do not leak into each other.
func Install(b *Book) {
	if b == nil {
		b = New(nil)
	}
	sharedMu.Lock()
	shared = b
	sharedMu.Unlock()
}

func dataKey(userID int64) string {
	return keyPrefix + strconv.FormatInt(userID, 10)
}

// Load returns the stored audience and the current generation. A missing
// list is a zero record with that generation and a nil error. A store error
// is returned so the caller still fans out instead of skipping the renewal.
func (b *Book) Load(ctx context.Context, userID int64) (Record, error) {
	if b == nil || b.be == nil || userID == 0 {
		return Record{}, nil
	}
	return b.be.load(ctx, userID)
}

// Save stores a newer expires and its audience when rec.Gen is still current.
// An older expires or a stale generation is left untouched. A list past
// MaxViewers is dropped and HaveList is false. A store error is returned;
// the push itself has already happened.
func (b *Book) Save(ctx context.Context, userID int64, rec Record) error {
	if b == nil || b.be == nil || userID == 0 || rec.Pushed <= 0 {
		return nil
	}
	if len(rec.Viewers) > MaxViewers {
		rec.Viewers = nil
		rec.HaveList = false
	}
	return b.be.save(ctx, userID, rec)
}

// Forget drops the list and bumps its generation so a save that already
// read the old list cannot write it back.
func (b *Book) Forget(ctx context.Context, userIDs ...int64) {
	if b == nil || b.be == nil {
		return
	}
	ids := make([]int64, 0, len(userIDs))
	seen := make(map[int64]struct{}, len(userIDs))
	for _, id := range userIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return
	}
	b.be.forget(ctx, ids)
}

// OpenRecord is the record a heartbeat may trust. A read error yields an
// empty record so the renewal still runs.
func OpenRecord(rec Record, err error) Record {
	if err != nil {
		return Record{}
	}
	return rec
}

// StatusPrivacyForgetIDs is everyone whose cached audience may contain self
// after a privacyKeyStatusTimestamp change: self, users named in the rules,
// and both directions of self's contacts.
func StatusPrivacyForgetIDs(self int64, named, forward, reverse []int64) []int64 {
	seen := make(map[int64]struct{}, 1+len(named)+len(forward)+len(reverse))
	out := make([]int64, 0, 1+len(named)+len(forward)+len(reverse))
	add := func(id int64) {
		if id == 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	add(self)
	for _, id := range named {
		add(id)
	}
	for _, id := range forward {
		add(id)
	}
	for _, id := range reverse {
		add(id)
	}
	return out
}

type memSlot struct {
	gen     int64
	pushed  int64
	have    bool
	viewers []Viewer
	expire  time.Time
}

type mem struct {
	mu sync.Mutex
	m  map[int64]memSlot
}

func newMem() *mem { return &mem{m: map[int64]memSlot{}} }

func (m *mem) load(_ context.Context, userID int64) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.m[userID]
	if !s.expire.IsZero() && time.Now().After(s.expire) {
		s.pushed = 0
		s.have = false
		s.viewers = nil
		s.expire = time.Time{}
		m.m[userID] = s
	}
	if s.gen == 0 && s.pushed == 0 && !s.have {
		return Record{}, nil
	}
	return Record{Pushed: s.pushed, Viewers: append([]Viewer(nil), s.viewers...), HaveList: s.have, Gen: s.gen}, nil
}

func (m *mem) save(_ context.Context, userID int64, rec Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.m[userID]
	if s.gen != rec.Gen || rec.Pushed <= s.pushed {
		return nil
	}
	s.pushed = rec.Pushed
	s.have = rec.HaveList
	s.viewers = append([]Viewer(nil), rec.Viewers...)
	s.expire = time.Now().Add(time.Duration(TTL) * time.Second)
	m.m[userID] = s
	return nil
}

func (m *mem) forget(_ context.Context, userIDs []int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range userIDs {
		s := m.m[id]
		s.gen++
		s.pushed = 0
		s.have = false
		s.viewers = nil
		s.expire = time.Time{}
		m.m[id] = s
	}
}

type scriptKV interface {
	EvalCtx(ctx context.Context, script, key string, args ...any) (any, error)
	HgetallCtx(ctx context.Context, key string) (map[string]string, error)
}

type redisBE struct {
	kv scriptKV
}

func (r redisBE) load(ctx context.Context, userID int64) (Record, error) {
	fields, err := r.kv.HgetallCtx(ctx, dataKey(userID))
	if err != nil {
		return Record{}, err
	}
	gen, _ := strconv.ParseInt(fields["gen"], 10, 64)
	if fields["body"] == "" {
		return Record{Gen: gen}, nil
	}
	var decoded body
	if json.Unmarshal([]byte(fields["body"]), &decoded) != nil {
		return Record{Gen: gen}, nil
	}
	pushed, _ := strconv.ParseInt(fields["pushed"], 10, 64)
	return Record{Pushed: pushed, Viewers: decoded.Viewers, HaveList: decoded.HaveList, Gen: gen}, nil
}

func (r redisBE) save(ctx context.Context, userID int64, rec Record) error {
	raw, err := json.Marshal(body{Viewers: rec.Viewers, HaveList: rec.HaveList})
	if err != nil {
		return err
	}
	_, err = r.kv.EvalCtx(ctx, saveScript, dataKey(userID),
		strconv.FormatInt(rec.Gen, 10),
		strconv.FormatInt(rec.Pushed, 10),
		string(raw),
		TTL,
	)
	return err
}

func (r redisBE) forget(ctx context.Context, userIDs []int64) {
	for _, id := range userIDs {
		_, _ = r.kv.EvalCtx(ctx, forgetScript, dataKey(id))
	}
}
