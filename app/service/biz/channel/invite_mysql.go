package channel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func isDuplicate(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

// inviteSQL is one MySQL session. *sql.DB, *sql.Conn and *sql.Tx all match.
// Primary-link changes use a transaction on the connection that holds the lock.
type inviteSQL interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *mysqlStore) InsertInvite(ctx context.Context, inv *StoredInvite) error {
	return insertInviteRow(ctx, s.db, inv)
}

func (s *mysqlStore) UpdateInvite(ctx context.Context, inv *StoredInvite) error {
	return updateInviteRow(ctx, s.db, inv)
}

func (s *mysqlStore) GetInviteByLink(ctx context.Context, link string) (*StoredInvite, error) {
	return getInviteByLink(ctx, s.db, link)
}

func (s *mysqlStore) ListInvites(ctx context.Context, channelID int64) ([]*StoredInvite, error) {
	return listInviteRows(ctx, s.db, channelID)
}

type mysqlInviteTx struct {
	tx *sql.Tx
}

func (t mysqlInviteTx) InsertInvite(ctx context.Context, inv *StoredInvite) error {
	return insertInviteRow(ctx, t.tx, inv)
}

func (t mysqlInviteTx) UpdateInvite(ctx context.Context, inv *StoredInvite) error {
	return updateInviteRow(ctx, t.tx, inv)
}

func (t mysqlInviteTx) GetInviteByLink(ctx context.Context, link string) (*StoredInvite, error) {
	return getInviteByLink(ctx, t.tx, link)
}

func (t mysqlInviteTx) ListInvites(ctx context.Context, channelID int64) ([]*StoredInvite, error) {
	return listInviteRows(ctx, t.tx, channelID)
}

// WithInviteLock holds a named MySQL lock on one connection, then runs fn
// in a transaction on that same connection. GET_LOCK is session-scoped, so
// taking it on one pooled connection and releasing it on another would stick.
// The channels row is not locked: message sends already use FOR UPDATE on it.
func (s *mysqlStore) WithInviteLock(ctx context.Context, channelID int64, fn func(context.Context, inviteLocker) error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	name := fmt.Sprintf("tellomi:chinv:%d", channelID)
	var got sql.NullInt64
	if err = conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 15)`, name).Scan(&got); err != nil {
		return err
	}
	if !got.Valid || got.Int64 != 1 {
		return errInviteLock
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT RELEASE_LOCK(?)`, name)
	}()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(ctx, mysqlInviteTx{tx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func insertInviteRow(ctx context.Context, db inviteSQL, inv *StoredInvite) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO channel_invites
  (channel_id, admin_id, link, permanent, revoked, request_needed, start_date, expire_date, usage_limit, title, date2)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		inv.ChannelID, inv.AdminID, inv.Link, boolInt(inv.Permanent), boolInt(inv.Revoked), boolInt(inv.RequestNeeded),
		inv.StartDate, inv.ExpireDate, inv.UsageLimit, inv.Title, inv.Date)
	if isDuplicate(err) {
		return errInviteExists
	}
	return err
}

func updateInviteRow(ctx context.Context, db inviteSQL, inv *StoredInvite) error {
	_, err := db.ExecContext(ctx, `
UPDATE channel_invites SET admin_id=?, permanent=?, revoked=?, request_needed=?, start_date=?, expire_date=?,
  usage_limit=?, title=?, date2=? WHERE channel_id=? AND link=?`,
		inv.AdminID, boolInt(inv.Permanent), boolInt(inv.Revoked), boolInt(inv.RequestNeeded),
		inv.StartDate, inv.ExpireDate, inv.UsageLimit, inv.Title, inv.Date, inv.ChannelID, inv.Link)
	return err
}

func getInviteByLink(ctx context.Context, db inviteSQL, link string) (*StoredInvite, error) {
	row := db.QueryRowContext(ctx, `
SELECT id, channel_id, admin_id, link, permanent, revoked, request_needed, start_date, expire_date, usage_limit, title, date2
FROM channel_invites WHERE link=?`, link)
	inv, err := scanInvite(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return inv, err
}

func listInviteRows(ctx context.Context, db inviteSQL, channelID int64) ([]*StoredInvite, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, channel_id, admin_id, link, permanent, revoked, request_needed, start_date, expire_date, usage_limit, title, date2
FROM channel_invites WHERE channel_id=? ORDER BY date2 DESC, id DESC`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*StoredInvite
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (s *mysqlStore) DeleteInvite(ctx context.Context, channelID int64, link string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM channel_invite_importers WHERE channel_id=? AND link=?`, channelID, link); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM channel_invites WHERE channel_id=? AND link=?`, channelID, link)
	return err
}

func (s *mysqlStore) DeleteRevokedInvites(ctx context.Context, channelID, adminID int64) error {
	if _, err := s.db.ExecContext(ctx, `
DELETE FROM channel_invite_importers WHERE channel_id=? AND link IN (
  SELECT link FROM (SELECT link FROM channel_invites WHERE channel_id=? AND admin_id=? AND revoked=1) revoked_links
)`, channelID, channelID, adminID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM channel_invites WHERE channel_id=? AND admin_id=? AND revoked=1`, channelID, adminID)
	return err
}

func (s *mysqlStore) UpsertImporter(ctx context.Context, row *StoredImporter) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO channel_invite_importers (channel_id, link, user_id, requested, approved_by, about, date2)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE channel_id=VALUES(channel_id), requested=VALUES(requested), approved_by=VALUES(approved_by), about=VALUES(about), date2=VALUES(date2)`,
		row.ChannelID, row.Link, row.UserID, boolInt(row.Requested), row.ApprovedBy, row.About, row.Date)
	return err
}

func (s *mysqlStore) ListImporters(ctx context.Context, channelID int64) ([]*StoredImporter, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, channel_id, link, user_id, requested, approved_by, about, date2
FROM channel_invite_importers WHERE channel_id=? ORDER BY date2 DESC, user_id DESC`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*StoredImporter
	for rows.Next() {
		row, err := scanImporter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *mysqlStore) DeleteImporter(ctx context.Context, link string, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM channel_invite_importers WHERE link=? AND user_id=?`, link, userID)
	return err
}

type inviteScanner interface {
	Scan(dest ...any) error
}

func scanInvite(row inviteScanner) (*StoredInvite, error) {
	var inv StoredInvite
	var permanent, revoked, request int
	err := row.Scan(&inv.ID, &inv.ChannelID, &inv.AdminID, &inv.Link, &permanent, &revoked, &request,
		&inv.StartDate, &inv.ExpireDate, &inv.UsageLimit, &inv.Title, &inv.Date)
	if err != nil {
		return nil, err
	}
	inv.Permanent = permanent != 0
	inv.Revoked = revoked != 0
	inv.RequestNeeded = request != 0
	return &inv, nil
}

func scanImporter(row inviteScanner) (*StoredImporter, error) {
	var out StoredImporter
	var requested int
	err := row.Scan(&out.ID, &out.ChannelID, &out.Link, &out.UserID, &requested, &out.ApprovedBy, &out.About, &out.Date)
	if err != nil {
		return nil, err
	}
	out.Requested = requested != 0
	return &out, nil
}
