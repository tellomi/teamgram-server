package profile

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/mysqlpool"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
	"google.golang.org/protobuf/proto"
)

// schemaSQL matches teamgramd/deploy/sql/migrate-20261130-business-profile.sql.
// Creating it again does not change rows that are already there.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS ` + "`business_profile`" + ` (
  ` + "`doc_key`" + ` varchar(191) NOT NULL,
  ` + "`data`" + ` mediumblob NOT NULL,
  PRIMARY KEY (` + "`doc_key`" + `)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
`

type docDB interface {
	Load(ctx context.Context, key string) ([]byte, bool, error)
	Save(ctx context.Context, key string, raw []byte) error
	Delete(ctx context.Context, key string) error
}

type sqlDocs struct{ db *sql.DB }

func (s sqlDocs) Load(ctx context.Context, key string) ([]byte, bool, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT data FROM business_profile WHERE doc_key=?`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func (s sqlDocs) Save(ctx context.Context, key string, raw []byte) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO business_profile (doc_key, data) VALUES (?,?)
ON DUPLICATE KEY UPDATE data=VALUES(data)`, key, raw)
	return err
}

func (s sqlDocs) Delete(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM business_profile WHERE doc_key=?`, key)
	return err
}

// UseDSN opens MySQL when dsn or TEAMGRAM_MYSQL_DSN is set. The shared book
// keeps its pointer so handlers that already captured Shared() see the store.
func UseDSN(dsn string) error {
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	if dsn == "" {
		return nil
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = mysqlpool.Discard(db)
		return err
	}
	if _, err = db.ExecContext(ctx, schemaSQL); err != nil {
		_ = mysqlpool.Discard(db)
		return err
	}
	shared.mu.Lock()
	shared.db = sqlDocs{db: db}
	shared.mu.Unlock()
	return nil
}

func userKey(id int64) string {
	return "user:" + strconv.FormatInt(id, 10)
}

func slugKey(slug string) string {
	return "slug:" + slug
}

type storedLink struct {
	Slug     string   `json:"slug"`
	Message  string   `json:"message"`
	Entities [][]byte `json:"entities,omitempty"`
	Title    string   `json:"title,omitempty"`
	HasTitle bool     `json:"has_title,omitempty"`
	Views    int32    `json:"views,omitempty"`
}

type storedPeer struct {
	LastActivity int32  `json:"last_activity,omitempty"`
	GreetingSent int32  `json:"greeting_sent,omitempty"`
	GreetedPrior int32  `json:"greeted_prior,omitempty"`
	AwaySent     int32  `json:"away_sent,omitempty"`
	AwayWindow   string `json:"away_window,omitempty"`
}

type storedUser struct {
	Hours       []byte               `json:"hours,omitempty"`
	Location    []byte               `json:"location,omitempty"`
	Greeting    []byte               `json:"greeting,omitempty"`
	Away        []byte               `json:"away,omitempty"`
	Intro       []byte               `json:"intro,omitempty"`
	Links       []storedLink         `json:"links,omitempty"`
	Peers       map[int64]storedPeer `json:"peers,omitempty"`
	AwayVersion int32                `json:"away_version,omitempty"`
}

// dbOpTimeout bounds one MySQL call. Profile reads and writes must not wait
// on a stuck server with context.Background.
const dbOpTimeout = 3 * time.Second

func dbContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), dbOpTimeout)
}

func (b *Book) userGate(user int64) *sync.Mutex {
	if v, ok := b.gates.Load(user); ok {
		return v.(*sync.Mutex)
	}
	m := &sync.Mutex{}
	actual, _ := b.gates.LoadOrStore(user, m)
	return actual.(*sync.Mutex)
}

func (b *Book) store() docDB {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.db
}

// fetchUser reads one profile outside mu. enabled is false when MySQL is off,
// and the in-memory row must be left alone.
func (b *Book) fetchUser(user int64) (raw []byte, found, enabled bool, err error) {
	db := b.store()
	if db == nil {
		return nil, false, false, nil
	}
	ctx, cancel := dbContext()
	defer cancel()
	raw, found, err = db.Load(ctx, userKey(user))
	return raw, found, true, err
}

// applyFetched installs a row loaded by fetchUser. Caller holds mu.
func (b *Book) applyFetched(user int64, raw []byte, found, enabled bool) error {
	if !enabled {
		return nil
	}
	b.forgetSlugs(user)
	if !found || len(raw) == 0 {
		delete(b.users, user)
		return nil
	}
	r, err := decodeUser(raw)
	if err != nil {
		return err
	}
	b.users[user] = r
	for _, item := range r.links {
		b.slugs[item.slug] = user
	}
	return nil
}

type userSnap struct {
	user      int64
	key       string
	deleteDoc bool
	raw       []byte
	slugs     []string
	dropSlugs []string
}

// snapshot copies the in-memory row for a write that happens after mu is released.
func (b *Book) snapshot(user int64, drop []string) (*userSnap, error) {
	snap := &userSnap{user: user, key: userKey(user), dropSlugs: append([]string(nil), drop...)}
	r := b.users[user]
	if r == nil || r.blank() {
		snap.deleteDoc = true
		return snap, nil
	}
	raw, err := encodeUser(r)
	if err != nil {
		return nil, err
	}
	snap.raw = raw
	for _, item := range r.links {
		snap.slugs = append(snap.slugs, item.slug)
	}
	return snap, nil
}

func (b *Book) writeSnap(snap *userSnap) error {
	if snap == nil {
		return nil
	}
	db := b.store()
	if db == nil {
		return nil
	}
	for _, slug := range snap.dropSlugs {
		ctx, cancel := dbContext()
		err := db.Delete(ctx, slugKey(slug))
		cancel()
		if err != nil {
			return err
		}
	}
	ctx, cancel := dbContext()
	var err error
	if snap.deleteDoc {
		err = db.Delete(ctx, snap.key)
	} else {
		err = db.Save(ctx, snap.key, snap.raw)
	}
	cancel()
	if err != nil {
		return err
	}
	for _, slug := range snap.slugs {
		ctx, cancel = dbContext()
		err = db.Save(ctx, slugKey(slug), []byte(strconv.FormatInt(snap.user, 10)))
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

// update reloads one user, runs fn while mu is held, then writes MySQL after
// releasing mu. The per-user gate keeps two replies from both deciding to send.
// fn may append slugs that the following write deletes.
func (b *Book) update(user int64, fn func(drop *[]string) error) error {
	if b == nil {
		return nil
	}
	g := b.userGate(user)
	g.Lock()
	defer g.Unlock()
	raw, found, enabled, err := b.fetchUser(user)
	if err != nil {
		return err
	}
	b.mu.Lock()
	if err = b.applyFetched(user, raw, found, enabled); err != nil {
		b.mu.Unlock()
		return err
	}
	var drop []string
	err = fn(&drop)
	var snap *userSnap
	if err == nil {
		snap, err = b.snapshot(user, drop)
	}
	b.mu.Unlock()
	if err != nil {
		return err
	}
	return b.writeSnap(snap)
}

func (b *Book) readSlug(slug string) (int64, bool, error) {
	if slug == "" {
		return 0, false, nil
	}
	db := b.store()
	if db == nil {
		return 0, false, nil
	}
	ctx, cancel := dbContext()
	defer cancel()
	raw, ok, err := db.Load(ctx, slugKey(slug))
	if err != nil || !ok || len(raw) == 0 {
		return 0, false, err
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id == 0 {
		return 0, false, nil
	}
	return id, true, nil
}

func (b *Book) forgetSlugs(user int64) {
	for slug, owner := range b.slugs {
		if owner == user {
			delete(b.slugs, slug)
		}
	}
}

func (r *row) blank() bool {
	if r == nil {
		return true
	}
	return !r.any() && len(r.links) == 0 && len(r.peers) == 0 && r.awayVersion == 0
}

func encodeUser(r *row) ([]byte, error) {
	doc := storedUser{AwayVersion: r.awayVersion}
	var err error
	if r.hours != nil {
		if doc.Hours, err = proto.Marshal(r.hours); err != nil {
			return nil, err
		}
	}
	if r.location != nil {
		if doc.Location, err = proto.Marshal(r.location); err != nil {
			return nil, err
		}
	}
	if r.greeting != nil {
		if doc.Greeting, err = proto.Marshal(r.greeting); err != nil {
			return nil, err
		}
	}
	if r.away != nil {
		if doc.Away, err = proto.Marshal(r.away); err != nil {
			return nil, err
		}
	}
	if r.intro != nil {
		if doc.Intro, err = proto.Marshal(r.intro); err != nil {
			return nil, err
		}
	}
	for _, item := range r.links {
		link := storedLink{
			Slug: item.slug, Message: item.message, Title: item.title,
			HasTitle: item.hasTitle, Views: item.views,
		}
		for _, ent := range item.entities {
			if ent == nil {
				continue
			}
			raw, err := proto.Marshal(ent)
			if err != nil {
				return nil, err
			}
			link.Entities = append(link.Entities, raw)
		}
		doc.Links = append(doc.Links, link)
	}
	if len(r.peers) > 0 {
		doc.Peers = map[int64]storedPeer{}
		for id, note := range r.peers {
			if note == nil {
				continue
			}
			doc.Peers[id] = storedPeer{
				LastActivity: note.LastActivity,
				GreetingSent: note.GreetingSent,
				GreetedPrior: note.GreetedPrior,
				AwaySent:     note.AwaySent,
				AwayWindow:   note.AwayWindow,
			}
		}
	}
	return json.Marshal(doc)
}

func decodeUser(raw []byte) (*row, error) {
	var doc storedUser
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	r := &row{
		peers:       map[int64]*peerNote{},
		awayVersion: doc.AwayVersion,
	}
	var err error
	if r.hours, err = unmarshalHours(doc.Hours); err != nil {
		return nil, err
	}
	if r.location, err = unmarshalLoc(doc.Location); err != nil {
		return nil, err
	}
	if r.greeting, err = unmarshalGreeting(doc.Greeting); err != nil {
		return nil, err
	}
	if r.away, err = unmarshalAway(doc.Away); err != nil {
		return nil, err
	}
	if r.intro, err = unmarshalIntro(doc.Intro); err != nil {
		return nil, err
	}
	for _, item := range doc.Links {
		link := &linkRow{
			slug: item.Slug, message: item.Message, title: item.Title,
			hasTitle: item.HasTitle, views: item.Views,
		}
		for _, rawEnt := range item.Entities {
			ent := &mtproto.MessageEntity{}
			if err := proto.Unmarshal(rawEnt, ent); err != nil {
				return nil, err
			}
			link.entities = append(link.entities, ent)
		}
		r.links = append(r.links, link)
	}
	for id, note := range doc.Peers {
		cp := note
		r.peers[id] = &peerNote{
			LastActivity: cp.LastActivity,
			GreetingSent: cp.GreetingSent,
			GreetedPrior: cp.GreetedPrior,
			AwaySent:     cp.AwaySent,
			AwayWindow:   cp.AwayWindow,
		}
	}
	return r, nil
}

func unmarshalHours(raw []byte) (*mtproto.BusinessWorkHours, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := &mtproto.BusinessWorkHours{}
	if err := proto.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	return out, nil
}

func unmarshalLoc(raw []byte) (*mtproto.BusinessLocation, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := &mtproto.BusinessLocation{}
	if err := proto.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	return out, nil
}

func unmarshalGreeting(raw []byte) (*mtproto.BusinessGreetingMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := &mtproto.BusinessGreetingMessage{}
	if err := proto.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	return out, nil
}

func unmarshalAway(raw []byte) (*mtproto.BusinessAwayMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := &mtproto.BusinessAwayMessage{}
	if err := proto.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	return out, nil
}

func unmarshalIntro(raw []byte) (*mtproto.BusinessIntro, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := &mtproto.BusinessIntro{}
	if err := proto.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	return out, nil
}
