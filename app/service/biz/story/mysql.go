package story

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/teamgram/proto/mtproto"
)

type mysqlStore struct {
	db     *sql.DB
	mu     sync.Mutex
	tables map[string]bool
}

// errAdvertSchema means the gated advertisement tables or the users index
// are not present. Callers must not fall back to a stories or users scan.
var errAdvertSchema = errors.New("story: advert schema is not migrated")

func newMySQLStore(db *sql.DB) Store {
	return &mysqlStore{db: db}
}

func (s *mysqlStore) alloc(ctx context.Context, owner Owner, column string) (int32, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	q := `INSERT INTO story_counters (owner_kind, owner_id, ` + column + `) VALUES (?, ?, LAST_INSERT_ID(1))
ON DUPLICATE KEY UPDATE ` + column + ` = LAST_INSERT_ID(` + column + ` + 1)`
	if _, err = tx.ExecContext(ctx, q, owner.Kind, owner.ID); err != nil {
		return 0, err
	}
	var id int64
	if err = tx.QueryRowContext(ctx, `SELECT LAST_INSERT_ID()`).Scan(&id); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return int32(id), nil
}

func (s *mysqlStore) Create(ctx context.Context, rec *Record) error {
	if rec.Item.Id == 0 {
		id, err := s.alloc(ctx, rec.Owner, "next_story")
		if err != nil {
			return err
		}
		rec.Item.Id = id
	}
	return s.upsert(ctx, rec)
}

func (s *mysqlStore) Save(ctx context.Context, rec *Record) error {
	if rec == nil || rec.id() == 0 {
		return errStoryIDInvalid
	}
	return s.upsert(ctx, rec)
}

func (s *mysqlStore) upsert(ctx context.Context, rec *Record) error {
	return upsertExec(ctx, s.db, rec)
}

type sqlExec interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func upsertExec(ctx context.Context, ex sqlExec, rec *Record) error {
	if rec.Item != nil {
		rec.Item.SanitizeEntities()
	}
	body, err := proto.Marshal(rec.Item)
	if err != nil {
		return err
	}
	_, err = ex.ExecContext(ctx, `
INSERT INTO stories (
  owner_kind, owner_id, story_id, sender_user_id, random_id, date2, expire_date, pinned, deleted, forwards, body)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE sender_user_id=VALUES(sender_user_id), random_id=VALUES(random_id),
  date2=VALUES(date2), expire_date=VALUES(expire_date), pinned=VALUES(pinned), deleted=VALUES(deleted),
  forwards=VALUES(forwards), body=VALUES(body)`,
		rec.Owner.Kind, rec.Owner.ID, rec.Item.Id, rec.Sender, rec.RandomID, rec.Item.Date, rec.Item.ExpireDate,
		boolInt(rec.Item.Pinned), boolInt(rec.Deleted), rec.Forwards, body)
	return err
}

// errNoAdvertUser means the story change was committed and there is no users
// row to store stories_max_id on.
var errNoAdvertUser = errors.New("story: user row missing")

// commitUserAdvert locks one users row, applies mutate, then sets
// stories_max_id from the live story rows in that same transaction.
// A second process blocks on the row until this commit, then recomputes,
// so it cannot store an older max. A crash before commit rolls the story
// change back with the advertisement.
func (s *mysqlStore) hasTable(ctx context.Context, name string) (bool, error) {
	s.mu.Lock()
	if s.tables[name] {
		s.mu.Unlock()
		return true, nil
	}
	s.mu.Unlock()
	var n int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, name).Scan(&n)
	if err != nil || n == 0 {
		return false, err
	}
	s.mu.Lock()
	if s.tables == nil {
		s.tables = map[string]bool{}
	}
	s.tables[name] = true
	s.mu.Unlock()
	return true, nil
}

func (s *mysqlStore) commitUserAdvert(ctx context.Context, userID int64, now int32, mutate func(*sql.Tx) error) (max int32, wrote bool, err error) {
	dirty, err := s.hasTable(ctx, "story_advert_dirty")
	if err != nil {
		return 0, false, err
	}
	live, err := s.hasTable(ctx, "story_live_owner")
	if err != nil {
		return 0, false, err
	}
	if !dirty || !live {
		// Do not write the story. A missing side table would leave the
		// advertisement without a bounded repair row.
		return 0, false, errAdvertSchema
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=? FOR UPDATE`, userID).Scan(&id)
	if err == sql.ErrNoRows {
		if mutate != nil {
			if err = mutate(tx); err != nil {
				return 0, false, err
			}
		}
		if err = tx.Commit(); err != nil {
			return 0, false, err
		}
		return 0, false, errNoAdvertUser
	}
	if err != nil {
		return 0, false, err
	}
	if mutate != nil {
		if err = mutate(tx); err != nil {
			return 0, false, err
		}
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE users SET stories_max_id = (
  SELECT IFNULL(MAX(story_id), 0) FROM stories
  WHERE owner_kind=? AND owner_id=? AND deleted=0 AND expire_date>?
) WHERE id=?`, KindUser, userID, now, userID); err != nil {
		return 0, false, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT stories_max_id FROM users WHERE id=?`, userID).Scan(&max); err != nil {
		return 0, false, err
	}
	// Same transaction as the column. A crash after commit leaves this row
	// so the next process can drop the user cache. The cache itself is not
	// deleted here.
	if _, err = tx.ExecContext(ctx, `
INSERT INTO story_advert_dirty (user_id, stories_max_id, updated_at)
VALUES (?,?,?)
ON DUPLICATE KEY UPDATE stories_max_id=?, updated_at=?`,
		userID, max, now, max, now); err != nil {
		return 0, false, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM story_live_owner WHERE owner_id=?`, userID); err != nil {
		return 0, false, err
	}
	if max > 0 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO story_live_owner (owner_id, live_max) VALUES (?, ?)`, userID, max); err != nil {
			return 0, false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, false, err
	}
	return max, true, nil
}

type advertDirty struct {
	userID int64
	max    int32
}

type staleAdvert struct {
	userID int64
	max    int32
}

func (s *mysqlStore) usersStoriesMaxIndexed(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND INDEX_NAME = 'users_stories_max'`).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// pageStaleAdverts reads the next non-zero advertisements after the cursor.
// It runs only when users_stories_max exists. Without that index the
// predicate is not executed.
func (s *mysqlStore) pageStaleAdverts(ctx context.Context, afterMax int32, afterID int64, limit int) ([]staleAdvert, error) {
	indexed, err := s.usersStoriesMaxIndexed(ctx)
	if err != nil {
		return nil, err
	}
	if !indexed {
		return nil, errAdvertSchema
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, stories_max_id FROM users
WHERE stories_max_id > ? OR (stories_max_id = ? AND id > ?)
ORDER BY stories_max_id, id
LIMIT ?`, afterMax, afterMax, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []staleAdvert
	for rows.Next() {
		var row staleAdvert
		if err = rows.Scan(&row.userID, &row.max); err != nil {
			return nil, err
		}
		if row.max <= 0 {
			continue
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type liveOwner struct {
	id  int64
	max int32
}

// pageLiveOwners reads the next rows of story_live_owner by primary key.
// One page is a range of limit keys. It does not read the stories table.
// A missing table returns errAdvertSchema instead of scanning stories.
func (s *mysqlStore) pageLiveOwners(ctx context.Context, afterOwner int64, limit int) ([]liveOwner, error) {
	ok, err := s.hasTable(ctx, "story_live_owner")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errAdvertSchema
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT owner_id, live_max FROM story_live_owner
WHERE owner_id > ?
ORDER BY owner_id
LIMIT ?`, afterOwner, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []liveOwner
	for rows.Next() {
		var row liveOwner
		if err = rows.Scan(&row.id, &row.max); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *mysqlStore) pageDirty(ctx context.Context, afterUser int64, limit int) ([]advertDirty, error) {
	ok, err := s.hasTable(ctx, "story_advert_dirty")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errAdvertSchema
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT user_id, stories_max_id FROM story_advert_dirty
WHERE user_id > ?
ORDER BY user_id
LIMIT ?`, afterUser, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []advertDirty
	for rows.Next() {
		var row advertDirty
		if err = rows.Scan(&row.userID, &row.max); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *mysqlStore) liveMax(ctx context.Context, userID int64, now int32) (int32, error) {
	var max int32
	err := s.db.QueryRowContext(ctx, `
SELECT IFNULL(MAX(story_id), 0) FROM stories
WHERE owner_kind=? AND owner_id=? AND deleted=0 AND expire_date>?`, KindUser, userID, now).Scan(&max)
	return max, err
}

func (s *mysqlStore) clearDirty(ctx context.Context, userID int64, max int32) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM story_advert_dirty WHERE user_id=? AND stories_max_id=?`, userID, max)
	return err
}

func scanRecord(row interface{ Scan(...any) error }) (*Record, error) {
	var (
		kind, pinned, deleted      int
		ownerID, sender, random    int64
		id, date, expire, forwards int32
		body                       []byte
	)
	if err := row.Scan(&kind, &ownerID, &id, &sender, &random, &date, &expire, &pinned, &deleted, &forwards, &body); err != nil {
		return nil, err
	}
	item := &mtproto.StoryItem{}
	if err := proto.Unmarshal(body, item); err != nil {
		return nil, err
	}
	item.SanitizeEntities()
	item.Id = id
	item.Date = date
	item.ExpireDate = expire
	item.Pinned = pinned != 0
	return &Record{
		Owner:    Owner{Kind: Kind(kind), ID: ownerID},
		Sender:   sender,
		RandomID: random,
		Forwards: forwards,
		Deleted:  deleted != 0,
		Item:     item,
	}, nil
}

const storyCols = `owner_kind, owner_id, story_id, sender_user_id, random_id, date2, expire_date, pinned, deleted, forwards, body`

func (s *mysqlStore) Get(ctx context.Context, owner Owner, id int32) (*Record, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+storyCols+` FROM stories WHERE owner_kind=? AND owner_id=? AND story_id=?`, owner.Kind, owner.ID, id)
	rec, err := scanRecord(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return rec, err
}

func (s *mysqlStore) FindByRandom(ctx context.Context, owner Owner, sender, randomID int64) (*Record, error) {
	if randomID == 0 {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx, `SELECT `+storyCols+` FROM stories WHERE owner_kind=? AND owner_id=? AND sender_user_id=? AND random_id=? AND deleted=0`,
		owner.Kind, owner.ID, sender, randomID)
	rec, err := scanRecord(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return rec, err
}

func (s *mysqlStore) list(ctx context.Context, q string, args ...any) ([]*Record, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Record
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id() < out[j].id() })
	return out, rows.Err()
}

func (s *mysqlStore) ListByOwner(ctx context.Context, owner Owner) ([]*Record, error) {
	return s.list(ctx, `SELECT `+storyCols+` FROM stories WHERE owner_kind=? AND owner_id=?`, owner.Kind, owner.ID)
}

// ListActiveByOwners reads every non-deleted story that has not expired.
// now is the service clock, which is UNIX_TIMESTAMP in production, so a test
// can move expiry without waiting.
func (s *mysqlStore) ListActiveByOwners(ctx context.Context, kind Kind, ids []int64, now int32) ([]*Record, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	const chunk = 200
	var out []*Record
	for start := 0; start < len(ids); start += chunk {
		end := start + chunk
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[start:end]
		args := make([]any, 0, len(part)+2)
		args = append(args, kind)
		for _, id := range part {
			args = append(args, id)
		}
		args = append(args, now)
		q := `SELECT ` + storyCols + ` FROM stories WHERE owner_kind=? AND owner_id IN (` + mysqlPlaceholders(len(part)) + `) AND deleted=0 AND expire_date>?`
		recs, err := s.list(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		out = append(out, recs...)
	}
	return out, nil
}

// pageActive reads the next non-deleted, unexpired stories of one kind.
// The cursor is (owner_id, story_id), which matches the primary key after
// owner_kind. expire_date is applied so expired rows are not paged.
func (s *mysqlStore) pageActive(ctx context.Context, kind Kind, now int32, afterOwner int64, afterStory int32, limit int) ([]*Record, error) {
	if limit <= 0 {
		return nil, nil
	}
	return s.list(ctx, `SELECT `+storyCols+` FROM stories
WHERE owner_kind=? AND deleted=0 AND expire_date>? AND (owner_id>? OR (owner_id=? AND story_id>?))
ORDER BY owner_id, story_id
LIMIT ?`, kind, now, afterOwner, afterOwner, afterStory, limit)
}

func mysqlPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

func (s *mysqlStore) closeFriendIDs(ctx context.Context, owner int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT contact_user_id FROM user_contacts
WHERE owner_user_id=? AND close_friend<>0 AND is_deleted=0`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// dialogPeers pages users who have a private dialog with owner.
// peer_type 2 is a user. The scan stops at limit rows.
func (s *mysqlStore) dialogPeers(ctx context.Context, owner int64, limit int) ([]int64, error) {
	if limit <= 0 {
		return nil, nil
	}
	const page = 500
	var out []int64
	var after int64
	for len(out) < limit {
		n := page
		if remain := limit - len(out); remain < n {
			n = remain
		}
		rows, err := s.db.QueryContext(ctx, `
SELECT user_id FROM dialogs
WHERE peer_type=? AND peer_id=? AND deleted=0 AND user_id>?
ORDER BY user_id
LIMIT ?`, mtproto.PEER_USER, owner, after, n)
		if err != nil {
			return nil, err
		}
		var batch []int64
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			batch = append(batch, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 || batch[len(batch)-1] <= after {
			break
		}
		out = append(out, batch...)
		after = batch[len(batch)-1]
		if len(batch) < n {
			break
		}
	}
	return out, nil
}

func (s *mysqlStore) ListAll(ctx context.Context) ([]*Record, error) {
	return s.list(ctx, `SELECT `+storyCols+` FROM stories`)
}

func (s *mysqlStore) OwnersExpiredBetween(ctx context.Context, after, now int32) ([]Owner, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT owner_kind, owner_id FROM stories
WHERE deleted=0 AND expire_date<=? AND expire_date>?`, now, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Owner
	for rows.Next() {
		var kind int
		var id int64
		if err = rows.Scan(&kind, &id); err != nil {
			return nil, err
		}
		out = append(out, Owner{Kind: Kind(kind), ID: id})
	}
	return out, rows.Err()
}

func (s *mysqlStore) NextExpiry(ctx context.Context, now int32) (int32, error) {
	var next sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT MIN(expire_date) FROM stories WHERE deleted=0 AND expire_date>?`, now).Scan(&next)
	if err != nil || !next.Valid {
		return 0, err
	}
	return int32(next.Int64), nil
}

func (s *mysqlStore) AddForwards(ctx context.Context, owner Owner, id int32) error {
	_, err := s.db.ExecContext(ctx, `UPDATE stories SET forwards=forwards+1 WHERE owner_kind=? AND owner_id=? AND story_id=?`, owner.Kind, owner.ID, id)
	return err
}

func (s *mysqlStore) SaveView(ctx context.Context, owner Owner, storyID int32, v View) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO story_views (owner_kind, owner_id, story_id, viewer_user_id, date2, reaction, stealth)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE date2=VALUES(date2), reaction=VALUES(reaction), stealth=VALUES(stealth)`,
		owner.Kind, owner.ID, storyID, v.UserID, v.Date, v.Reaction, boolInt(v.Stealth))
	return err
}

func (s *mysqlStore) ListViews(ctx context.Context, owner Owner, storyID int32) ([]View, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT viewer_user_id, date2, reaction, stealth FROM story_views WHERE owner_kind=? AND owner_id=? AND story_id=? ORDER BY date2 DESC, viewer_user_id`,
		owner.Kind, owner.ID, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []View
	for rows.Next() {
		var v View
		var stealth int
		if err = rows.Scan(&v.UserID, &v.Date, &v.Reaction, &stealth); err != nil {
			return nil, err
		}
		v.Stealth = stealth != 0
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *mysqlStore) HideActiveViews(ctx context.Context, viewer int64, now int32) error {
	_, err := s.db.ExecContext(ctx, `
DELETE v FROM story_views v
INNER JOIN stories s ON s.owner_kind=v.owner_kind AND s.owner_id=v.owner_id AND s.story_id=v.story_id
WHERE v.viewer_user_id=? AND s.deleted=0 AND s.expire_date>?`, viewer, now)
	return err
}

func (s *mysqlStore) GetRead(ctx context.Context, userID int64, owner Owner) (*Read, error) {
	var maxID int32
	var hidden int
	err := s.db.QueryRowContext(ctx, `SELECT max_read_id, hidden FROM story_reads WHERE user_id=? AND owner_kind=? AND owner_id=?`,
		userID, owner.Kind, owner.ID).Scan(&maxID, &hidden)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Read{UserID: userID, Owner: owner, MaxID: maxID, Hidden: hidden != 0}, nil
}

func (s *mysqlStore) SaveRead(ctx context.Context, rd *Read) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO story_reads (user_id, owner_kind, owner_id, max_read_id, hidden) VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE max_read_id=VALUES(max_read_id), hidden=VALUES(hidden)`,
		rd.UserID, rd.Owner.Kind, rd.Owner.ID, rd.MaxID, boolInt(rd.Hidden))
	return err
}

func (s *mysqlStore) ListReads(ctx context.Context, userID int64) ([]*Read, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT owner_kind, owner_id, max_read_id, hidden FROM story_reads WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Read
	for rows.Next() {
		rd := &Read{UserID: userID}
		var kind, hidden int
		if err = rows.Scan(&kind, &rd.Owner.ID, &rd.MaxID, &hidden); err != nil {
			return nil, err
		}
		rd.Owner.Kind = Kind(kind)
		rd.Hidden = hidden != 0
		out = append(out, rd)
	}
	return out, rows.Err()
}

func (s *mysqlStore) GetSettings(ctx context.Context, userID int64) (*Settings, error) {
	st := &Settings{UserID: userID}
	var hidden int
	err := s.db.QueryRowContext(ctx, `SELECT all_hidden, stealth_active, stealth_cooldown FROM story_settings WHERE user_id=?`, userID).
		Scan(&hidden, &st.StealthActive, &st.StealthCooldown)
	if err == sql.ErrNoRows {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	st.AllHidden = hidden != 0
	return st, nil
}

func (s *mysqlStore) SaveSettings(ctx context.Context, st *Settings) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO story_settings (user_id, all_hidden, stealth_active, stealth_cooldown) VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE all_hidden=VALUES(all_hidden), stealth_active=VALUES(stealth_active), stealth_cooldown=VALUES(stealth_cooldown)`,
		st.UserID, boolInt(st.AllHidden), st.StealthActive, st.StealthCooldown)
	return err
}

func joinIDs(ids []int32) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(int64(id), 10)
	}
	return strings.Join(parts, ",")
}

func splitIDs(s string) []int32 {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]int32, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.ParseInt(p, 10, 32)
		if err == nil && n != 0 {
			out = append(out, int32(n))
		}
	}
	return out
}

func (s *mysqlStore) CreateAlbum(ctx context.Context, a *Album) error {
	if a.ID == 0 {
		id, err := s.alloc(ctx, a.Owner, "next_album")
		if err != nil {
			return err
		}
		a.ID = id
	}
	if a.Order == 0 {
		var n int32
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM story_albums WHERE owner_kind=? AND owner_id=?`, a.Owner.Kind, a.Owner.ID).Scan(&n)
		a.Order = n + 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO story_albums (owner_kind, owner_id, album_id, title, order_index, story_ids) VALUES (?,?,?,?,?,?)`,
		a.Owner.Kind, a.Owner.ID, a.ID, a.Title, a.Order, joinIDs(a.StoryIDs))
	return err
}

func (s *mysqlStore) SaveAlbum(ctx context.Context, a *Album) error {
	res, err := s.db.ExecContext(ctx, `UPDATE story_albums SET title=?, order_index=?, story_ids=? WHERE owner_kind=? AND owner_id=? AND album_id=?`,
		a.Title, a.Order, joinIDs(a.StoryIDs), a.Owner.Kind, a.Owner.ID, a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var id int32
		err = s.db.QueryRowContext(ctx, `SELECT album_id FROM story_albums WHERE owner_kind=? AND owner_id=? AND album_id=?`,
			a.Owner.Kind, a.Owner.ID, a.ID).Scan(&id)
		if err == sql.ErrNoRows {
			return errAlbumInvalid
		}
		return err
	}
	return nil
}

func scanAlbum(row interface{ Scan(...any) error }) (*Album, error) {
	a := &Album{}
	var kind int
	var ids string
	if err := row.Scan(&kind, &a.Owner.ID, &a.ID, &a.Title, &a.Order, &ids); err != nil {
		return nil, err
	}
	a.Owner.Kind = Kind(kind)
	a.StoryIDs = splitIDs(ids)
	return a, nil
}

func (s *mysqlStore) GetAlbum(ctx context.Context, owner Owner, id int32) (*Album, error) {
	row := s.db.QueryRowContext(ctx, `SELECT owner_kind, owner_id, album_id, title, order_index, story_ids FROM story_albums WHERE owner_kind=? AND owner_id=? AND album_id=?`,
		owner.Kind, owner.ID, id)
	a, err := scanAlbum(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (s *mysqlStore) ListAlbums(ctx context.Context, owner Owner) ([]*Album, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT owner_kind, owner_id, album_id, title, order_index, story_ids FROM story_albums WHERE owner_kind=? AND owner_id=? ORDER BY order_index, album_id`,
		owner.Kind, owner.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Album
	for rows.Next() {
		a, err := scanAlbum(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *mysqlStore) DeleteAlbum(ctx context.Context, owner Owner, id int32) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM story_albums WHERE owner_kind=? AND owner_id=? AND album_id=?`, owner.Kind, owner.ID, id)
	return err
}

func (s *mysqlStore) SetPinnedTop(ctx context.Context, owner Owner, ids []int32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM story_pinned_top WHERE owner_kind=? AND owner_id=?`, owner.Kind, owner.ID); err != nil {
		return err
	}
	for i, id := range ids {
		if _, err = tx.ExecContext(ctx, `INSERT INTO story_pinned_top (owner_kind, owner_id, pos, story_id) VALUES (?,?,?,?)`,
			owner.Kind, owner.ID, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *mysqlStore) PinnedTop(ctx context.Context, owner Owner) ([]int32, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT story_id FROM story_pinned_top WHERE owner_kind=? AND owner_id=? ORDER BY pos`, owner.Kind, owner.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int32
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *mysqlStore) SaveReport(ctx context.Context, r *Report) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO story_reports (reporter_user_id, owner_kind, owner_id, story_ids, option_text, message, date2) VALUES (?,?,?,?,?,?,?)`,
		r.Reporter, r.Owner.Kind, r.Owner.ID, joinIDs(r.StoryIDs), r.Option, r.Message, r.Date)
	return err
}

func (s *mysqlStore) SaveDeletions(ctx context.Context, owner Owner, storyID, deletedAt int32, viewers []int64) error {
	if s == nil || s.db == nil || storyID == 0 || len(viewers) == 0 {
		return nil
	}
	cutoff := int64(deletedAt) - int64(deletionReplayWindow)
	if cutoff > 0 {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM story_deletions WHERE deleted_at < ? LIMIT 500`, cutoff); err != nil {
			return err
		}
	}
	const chunk = 200
	for start := 0; start < len(viewers); start += chunk {
		end := start + chunk
		if end > len(viewers) {
			end = len(viewers)
		}
		part := viewers[start:end]
		var b strings.Builder
		b.WriteString(`INSERT INTO story_deletions (viewer_user_id, owner_kind, owner_id, story_id, deleted_at) VALUES `)
		args := make([]any, 0, len(part)*5)
		for _, id := range part {
			if id == 0 {
				continue
			}
			if len(args) > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`(?,?,?,?,?)`)
			args = append(args, id, owner.Kind, owner.ID, storyID, deletedAt)
		}
		if len(args) == 0 {
			continue
		}
		b.WriteString(` ON DUPLICATE KEY UPDATE deleted_at=GREATEST(deleted_at, VALUES(deleted_at))`)
		if _, err := s.db.ExecContext(ctx, b.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *mysqlStore) ListDeletions(ctx context.Context, viewer int64, since int32, limit int) ([]Deletion, error) {
	if s == nil || s.db == nil || viewer == 0 || limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT owner_kind, owner_id, story_id, deleted_at
FROM story_deletions
WHERE viewer_user_id=? AND deleted_at>?
ORDER BY deleted_at DESC, owner_kind DESC, owner_id DESC, story_id DESC
LIMIT ?`, viewer, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deletion
	for rows.Next() {
		var row Deletion
		var kind int
		if err = rows.Scan(&kind, &row.Owner.ID, &row.StoryID, &row.At); err != nil {
			return nil, err
		}
		row.Owner.Kind = Kind(kind)
		out = append(out, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *mysqlStore) DeletionMarks(ctx context.Context, viewer int64) ([]DeletionMark, error) {
	if s == nil || s.db == nil || viewer == 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT owner_kind, owner_id, COUNT(*), MAX(story_id), MAX(deleted_at)
FROM story_deletions
WHERE viewer_user_id=?
GROUP BY owner_kind, owner_id`, viewer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeletionMark
	for rows.Next() {
		var mark DeletionMark
		var kind int
		if err = rows.Scan(&kind, &mark.Owner.ID, &mark.Count, &mark.MaxID, &mark.MaxAt); err != nil {
			return nil, err
		}
		mark.Owner.Kind = Kind(kind)
		out = append(out, mark)
	}
	return out, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
