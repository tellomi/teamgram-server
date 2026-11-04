package discoverprofile

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// Queries read existing tables. This feature adds no migration.
// Phone is not selected. Contact-book names are not selected.
const (
	userLookupSQL = `SELECT sub, user_id FROM discover_subject WHERE sub IN (`
	profileSQL    = `SELECT id, first_name, last_name, username, about, photo_id, deleted FROM users WHERE id IN (`
	privacySQL    = `SELECT user_id, key_type, rules FROM user_privacies WHERE key_type IN (` +
		keyPhotoSQL + "," + keyAboutSQL + `) AND user_id IN (`
	edgeSQL = `SELECT owner_user_id, close_friend FROM user_contacts
WHERE contact_user_id = ? AND is_deleted = 0 AND owner_user_id IN (`
)

const (
	keyPhotoSQL = "6"  // mtproto.PROFILE_PHOTO
	keyAboutSQL = "10" // mtproto.ABOUT
)

// MySQL reads the teamgram database. The DSN is not logged.
type MySQL struct {
	db *sql.DB
}

// OpenMySQL opens the teamgram database.
func OpenMySQL(dsn string) (*MySQL, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	return &MySQL{db: db}, nil
}

// Ping checks the connection.
func (m *MySQL) Ping(ctx context.Context) error {
	return m.db.PingContext(ctx)
}

// Users resolves raw subs. Missing subs are omitted.
func (m *MySQL) Users(ctx context.Context, subs [][]byte) (map[string]int64, error) {
	out := map[string]int64{}
	if len(subs) == 0 {
		return out, nil
	}
	args := make([]any, len(subs))
	for i, sub := range subs {
		args[i] = sub
	}
	rows, err := m.db.QueryContext(ctx, userLookupSQL+placeholders(len(subs))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sub []byte
		var id int64
		if err = rows.Scan(&sub, &id); err != nil {
			return nil, err
		}
		out[subKey(sub)] = id
	}
	return out, rows.Err()
}

// Profiles reads account names. A missing id is omitted.
func (m *MySQL) Profiles(ctx context.Context, ids []int64) (map[int64]Profile, error) {
	out := map[int64]Profile{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := m.db.QueryContext(ctx, profileSQL+placeholders(len(ids))+`)`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p Profile
		var deleted int
		if err = rows.Scan(&id, &p.FirstName, &p.LastName, &p.Username, &p.About, &p.PhotoID, &deleted); err != nil {
			return nil, err
		}
		p.Deleted = deleted != 0
		out[id] = p
	}
	return out, rows.Err()
}

// Privacy reads profile-photo and about rules. Other keys are not loaded.
func (m *MySQL) Privacy(ctx context.Context, ids []int64) (map[int64]Privacy, error) {
	out := map[int64]Privacy{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := m.db.QueryContext(ctx, privacySQL+placeholders(len(ids))+`)`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var key int32
		var raw sql.NullString
		if err = rows.Scan(&id, &key, &raw); err != nil {
			return nil, err
		}
		p := out[id]
		rules, broken := ParseRules(raw.String)
		switch key {
		case keyProfilePhoto:
			p.Photo, p.PhotoSet, p.PhotoBroken = rules, true, broken
		case keyAbout:
			p.About, p.AboutSet, p.AboutBroken = rules, true, broken
		}
		out[id] = p
	}
	return out, rows.Err()
}

// Edges returns authors who currently list viewer in user_contacts
// (is_deleted = 0). close_friend is the Stories flag on that row.
func (m *MySQL) Edges(ctx context.Context, viewer int64, authors []int64) (map[int64]Edge, error) {
	out := map[int64]Edge{}
	if viewer <= 0 || len(authors) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(authors)+1)
	args = append(args, viewer)
	for _, id := range authors {
		args = append(args, id)
	}
	rows, err := m.db.QueryContext(ctx, edgeSQL+placeholders(len(authors))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var author int64
		var closeFriend bool
		if err = rows.Scan(&author, &closeFriend); err != nil {
			return nil, err
		}
		out[author] = Edge{Contact: true, CloseFriend: closeFriend}
	}
	return out, rows.Err()
}

func idArgs(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

func init() {
	if strconv.Itoa(int(keyProfilePhoto)) != keyPhotoSQL || strconv.Itoa(int(keyAbout)) != keyAboutSQL {
		panic("discoverprofile: privacy key sql drifted from mtproto")
	}
}
