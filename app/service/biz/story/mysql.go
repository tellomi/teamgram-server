package story

import (
	"context"
	"database/sql"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/teamgram/proto/mtproto"
)

type mysqlStore struct {
	db *sql.DB
}

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
	if rec.Item != nil {
		rec.Item.SanitizeEntities()
	}
	body, err := proto.Marshal(rec.Item)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
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

func (s *mysqlStore) ListAll(ctx context.Context) ([]*Record, error) {
	return s.list(ctx, `SELECT `+storyCols+` FROM stories`)
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

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
