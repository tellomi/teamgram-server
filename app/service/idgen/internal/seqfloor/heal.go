// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package seqfloor

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/kv"

	_ "github.com/go-sql-driver/mysql"
)

// KV is the Redis surface the healer needs.
type KV interface {
	GetCtx(ctx context.Context, key string) (string, error)
	EvalCtx(ctx context.Context, script, key string, args ...any) (any, error)
}

// Cache remembers database floors and re-reads one key when Redis has lost it.
type Cache struct {
	db     *sql.DB
	mu     sync.Mutex
	floors map[string]int64
}

// Floor returns the last known database max for key. It does not hit MySQL.
func (c *Cache) Floor(key string) int64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.floors[key]
}

// Refresh re-reads one counter from MySQL and keeps the higher floor.
// Missing keys after a flush call this so a stale startup snapshot cannot
// sit below rows inserted later.
func (c *Cache) Refresh(ctx context.Context, key string) int64 {
	if c == nil || c.db == nil {
		return 0
	}
	live, ok, err := c.lookup(ctx, key)
	if err != nil {
		logx.WithContext(ctx).Errorf("seqfloor: refresh %s: %v", key, err)
		return c.Floor(key)
	}
	if !ok {
		return c.Floor(key)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if live > c.floors[key] {
		c.floors[key] = live
	}
	return c.floors[key]
}

// Close closes the MySQL connection used for refreshes.
func (c *Cache) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

// Heal loads MAX(id) for every sequence family and raises Redis counters that
// are missing or lower. Counters already above the database max are not
// changed. An empty dsn skips healing so local runs without MySQL still start;
// production yaml sets Mysql.DSN, and a configured database that cannot be
// read is a startup error.
func Heal(ctx context.Context, dsn string, store KV) (*Cache, error) {
	if strings.TrimSpace(dsn) == "" {
		logx.Error("seqfloor: Mysql.DSN is empty; id counters will not be healed from the database. A Redis flush will issue ids from 1 again.")
		return nil, nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("seqfloor: open mysql: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("seqfloor: ping mysql: %w", err)
	}
	floors, err := LoadFloors(ctx, db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	raised, kept, err := RaiseAll(ctx, store, floors)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	logx.Infof("seqfloor: database floors=%d raised=%d already-ahead=%d (qts has no mysql sequence; qts_updates_ngen_* stays wherever redis left it)", len(floors), raised, kept)
	return &Cache{db: db, floors: floors}, nil
}

// RaiseAll writes every floor that is above the Redis value.
func RaiseAll(ctx context.Context, store KV, floors map[string]int64) (raised, kept int, err error) {
	for key, floor := range floors {
		if floor <= 0 {
			continue
		}
		cur, getErr := store.GetCtx(ctx, key)
		if getErr != nil {
			return raised, kept, fmt.Errorf("seqfloor: get %s: %w", key, getErr)
		}
		if !ShouldRaise(cur, floor) {
			kept++
			continue
		}
		if _, evalErr := store.EvalCtx(ctx, raiseLua, key, floor); evalErr != nil {
			return raised, kept, fmt.Errorf("seqfloor: raise %s to %d: %w", key, floor, evalErr)
		}
		raised++
		logx.Infof("seqfloor: raised %s from %q to %d", key, cur, floor)
	}
	return raised, kept, nil
}

// Alloc issues the next n sequence values for key, raising a missing or low
// counter to the database floor first.
func Alloc(ctx context.Context, store KV, cache *Cache, key string, n int64) (int64, error) {
	if cache == nil {
		v, err := store.EvalCtx(ctx, allocLua, key, 0, n)
		if err != nil {
			// Fall back is not available; Eval is the increment.
			return 0, err
		}
		return asInt64(v)
	}
	floor := cache.Floor(key)
	cur, err := store.GetCtx(ctx, key)
	if err != nil {
		return 0, err
	}
	if cur == "" {
		if live := cache.Refresh(ctx, key); live > floor {
			floor = live
		}
	}
	v, err := store.EvalCtx(ctx, allocLua, key, floor, n)
	if err != nil {
		return 0, err
	}
	return asInt64(v)
}

// LoadFloors reads the highest stored id for each Redis sequence family.
func LoadFloors(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	out := map[string]int64{}
	put := func(prefix string, id, max int64) {
		if id == 0 || max <= 0 {
			return
		}
		key := prefix + strconv.FormatInt(id, 10)
		if max > out[key] {
			out[key] = max
		}
	}

	tables, err := messageTables(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, table := range tables {
		if err = queryPairs(ctx, db, "SELECT user_id, COALESCE(MAX(user_message_box_id),0) FROM "+quoteIdent(table)+" GROUP BY user_id", func(id, max int64) {
			put(MessageBoxPrefix, id, max)
		}); err != nil {
			return nil, fmt.Errorf("seqfloor: %s: %w", table, err)
		}
	}

	if ok, err := tableExists(ctx, db, "user_pts_updates"); err != nil {
		return nil, err
	} else if ok {
		if err = queryPairs(ctx, db, "SELECT user_id, COALESCE(MAX(pts),0) FROM user_pts_updates GROUP BY user_id", func(id, max int64) {
			put(PtsPrefix, id, max)
		}); err != nil {
			return nil, fmt.Errorf("seqfloor: user_pts_updates: %w", err)
		}
	}

	if ok, err := tableExists(ctx, db, "auth_seq_updates"); err != nil {
		return nil, err
	} else if ok {
		if err = queryPairs(ctx, db, "SELECT auth_id, COALESCE(MAX(seq),0) FROM auth_seq_updates GROUP BY auth_id", func(id, max int64) {
			put(SeqPrefix, id, max)
		}); err != nil {
			return nil, fmt.Errorf("seqfloor: auth_seq_updates: %w", err)
		}
	}

	if ok, err := tableExists(ctx, db, "channel_messages"); err != nil {
		return nil, err
	} else if ok {
		if err = queryPairs(ctx, db, "SELECT channel_id, COALESCE(MAX(message_id),0) FROM channel_messages GROUP BY channel_id", func(id, max int64) {
			put(ChannelMessageBoxPrefix, id, max)
		}); err != nil {
			return nil, fmt.Errorf("seqfloor: channel_messages: %w", err)
		}
	}
	if ok, err := tableExists(ctx, db, "channels"); err != nil {
		return nil, err
	} else if ok {
		if err = queryPairs(ctx, db, "SELECT id, COALESCE(top_message,0) FROM channels", func(id, max int64) {
			put(ChannelMessageBoxPrefix, id, max)
		}); err != nil {
			return nil, fmt.Errorf("seqfloor: channels.top_message: %w", err)
		}
		if err = queryPairs(ctx, db, "SELECT id, COALESCE(pts,0) FROM channels", func(id, max int64) {
			put(ChannelPtsPrefix, id, max)
		}); err != nil {
			return nil, fmt.Errorf("seqfloor: channels.pts: %w", err)
		}
	}
	if ok, err := tableExists(ctx, db, "channel_pts_updates"); err != nil {
		return nil, err
	} else if ok {
		if err = queryPairs(ctx, db, "SELECT channel_id, COALESCE(MAX(pts),0) FROM channel_pts_updates GROUP BY channel_id", func(id, max int64) {
			put(ChannelPtsPrefix, id, max)
		}); err != nil {
			return nil, fmt.Errorf("seqfloor: channel_pts_updates: %w", err)
		}
	}

	if ok, err := tableExists(ctx, db, "stories"); err != nil {
		return nil, err
	} else if ok {
		if err = queryPairs(ctx, db, "SELECT owner_id, COALESCE(MAX(story_id),0) FROM stories WHERE owner_kind=1 GROUP BY owner_id", func(id, max int64) {
			put(StoryPrefix, id, max)
		}); err != nil {
			return nil, fmt.Errorf("seqfloor: stories: %w", err)
		}
		if err = queryPairs(ctx, db, "SELECT owner_id, COALESCE(MAX(story_id),0) FROM stories WHERE owner_kind=2 GROUP BY owner_id", func(id, max int64) {
			put(ChannelStoryPrefix, id, max)
		}); err != nil {
			return nil, fmt.Errorf("seqfloor: channel stories: %w", err)
		}
	}

	// qts_updates_ngen_{authKeyId} is only in Redis. Secret-chat qts updates
	// are not written to MySQL, so a flush cannot be rebuilt from a MAX(qts).
	logx.Info("seqfloor: qts_updates_ngen_* has no database table; getDifference qts cannot be restored after a Redis flush")
	return out, nil
}

func (c *Cache) lookup(ctx context.Context, key string) (int64, bool, error) {
	switch {
	case strings.HasPrefix(key, MessageBoxPrefix):
		return c.maxOverTables(ctx, messageBoxQuery, parseSuffix(key, MessageBoxPrefix))
	case strings.HasPrefix(key, PtsPrefix):
		return c.scalar(ctx, "SELECT COALESCE(MAX(pts),0) FROM user_pts_updates WHERE user_id=?", parseSuffix(key, PtsPrefix))
	case strings.HasPrefix(key, SeqPrefix):
		return c.scalar(ctx, "SELECT COALESCE(MAX(seq),0) FROM auth_seq_updates WHERE auth_id=?", parseSuffix(key, SeqPrefix))
	case strings.HasPrefix(key, ChannelMessageBoxPrefix):
		id := parseSuffix(key, ChannelMessageBoxPrefix)
		msg, _, err := c.scalar(ctx, "SELECT COALESCE(MAX(message_id),0) FROM channel_messages WHERE channel_id=?", id)
		if err != nil && !isNoSuchTable(err) {
			return 0, false, err
		}
		top, _, err := c.scalar(ctx, "SELECT COALESCE(top_message,0) FROM channels WHERE id=?", id)
		if err != nil && !isNoSuchTable(err) {
			return 0, false, err
		}
		if top > msg {
			msg = top
		}
		return msg, msg > 0, nil
	case strings.HasPrefix(key, ChannelPtsPrefix):
		id := parseSuffix(key, ChannelPtsPrefix)
		pts, _, err := c.scalar(ctx, "SELECT COALESCE(MAX(pts),0) FROM channel_pts_updates WHERE channel_id=?", id)
		if err != nil && !isNoSuchTable(err) {
			return 0, false, err
		}
		ch, _, err := c.scalar(ctx, "SELECT COALESCE(pts,0) FROM channels WHERE id=?", id)
		if err != nil && !isNoSuchTable(err) {
			return 0, false, err
		}
		if ch > pts {
			pts = ch
		}
		return pts, pts > 0, nil
	case strings.HasPrefix(key, ChannelStoryPrefix):
		return c.scalar(ctx, "SELECT COALESCE(MAX(story_id),0) FROM stories WHERE owner_kind=2 AND owner_id=?", parseSuffix(key, ChannelStoryPrefix))
	case strings.HasPrefix(key, StoryPrefix):
		return c.scalar(ctx, "SELECT COALESCE(MAX(story_id),0) FROM stories WHERE owner_kind=1 AND owner_id=?", parseSuffix(key, StoryPrefix))
	default:
		return 0, false, nil
	}
}

func messageBoxQuery(table string) string {
	return "SELECT COALESCE(MAX(user_message_box_id),0) FROM " + quoteIdent(table) + " WHERE user_id=?"
}

func (c *Cache) maxOverTables(ctx context.Context, queryFor func(string) string, userID int64) (int64, bool, error) {
	tables, err := messageTables(ctx, c.db)
	if err != nil {
		return 0, false, err
	}
	var max int64
	for _, table := range tables {
		n, _, err := c.scalar(ctx, queryFor(table), userID)
		if err != nil {
			return 0, false, err
		}
		if n > max {
			max = n
		}
	}
	return max, max > 0, nil
}

func (c *Cache) scalar(ctx context.Context, query string, arg int64) (int64, bool, error) {
	var n sql.NullInt64
	err := c.db.QueryRowContext(ctx, query, arg).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		if isNoSuchTable(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if !n.Valid || n.Int64 <= 0 {
		return 0, false, nil
	}
	return n.Int64, true, nil
}

func queryPairs(ctx context.Context, db *sql.DB, query string, fn func(id, max int64)) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, max sql.NullInt64
		if err = rows.Scan(&id, &max); err != nil {
			return err
		}
		if id.Valid && max.Valid {
			fn(id.Int64, max.Int64)
		}
	}
	return rows.Err()
}

func messageTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT table_name FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND (table_name = 'messages' OR table_name REGEXP '^messages_[0-9]+$')
ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ?`, name).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "") + "`"
}

func parseSuffix(key, prefix string) int64 {
	id, err := strconv.ParseInt(strings.TrimPrefix(key, prefix), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func isNoSuchTable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "1146") || strings.Contains(msg, "doesn't exist")
}

func asInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int32:
		return int64(n), nil
	case int:
		return int64(n), nil
	case float64:
		return int64(n), nil
	case string:
		return strconv.ParseInt(n, 10, 64)
	case []byte:
		return strconv.ParseInt(string(n), 10, 64)
	default:
		return 0, fmt.Errorf("seqfloor: unexpected redis eval result %T", v)
	}
}

// Ensure kv.Store satisfies KV at compile time.
var _ KV = kv.Store(nil)
