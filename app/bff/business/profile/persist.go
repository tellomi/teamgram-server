package profile

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
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

func (b *Book) reload(user int64) error {
	if b == nil || b.db == nil {
		return nil
	}
	raw, ok, err := b.db.Load(context.Background(), userKey(user))
	if err != nil {
		return err
	}
	b.forgetSlugs(user)
	if !ok || len(raw) == 0 {
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

func (b *Book) save(user int64) error {
	if b == nil || b.db == nil {
		return nil
	}
	ctx := context.Background()
	r := b.users[user]
	if r == nil || r.blank() {
		return b.db.Delete(ctx, userKey(user))
	}
	raw, err := encodeUser(r)
	if err != nil {
		return err
	}
	if err = b.db.Save(ctx, userKey(user), raw); err != nil {
		return err
	}
	for _, item := range r.links {
		if err = b.db.Save(ctx, slugKey(item.slug), []byte(strconv.FormatInt(user, 10))); err != nil {
			return err
		}
	}
	return nil
}

func (b *Book) slugOwner(slug string) (int64, bool, error) {
	if b == nil || b.db == nil || slug == "" {
		return 0, false, nil
	}
	raw, ok, err := b.db.Load(context.Background(), slugKey(slug))
	if err != nil || !ok || len(raw) == 0 {
		return 0, false, err
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id == 0 {
		return 0, false, nil
	}
	b.slugs[slug] = id
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
