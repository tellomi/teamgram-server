package push

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Open connects to the teamgram database that holds `devices` and
// `user_notify_settings`. sql.Open does not dial; the first query does.
func Open(dsn string) (*MySQL, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("empty mysql dsn")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)
	return &MySQL{db: db}, nil
}

// MySQL is a Store and a settings/name lookup backed by the teamgram schema.
type MySQL struct {
	db *sql.DB
}

func (m *MySQL) Close() error { return m.db.Close() }

func (m *MySQL) Upsert(ctx context.Context, d Device) error {
	secret := ""
	if len(d.Secret) > 0 {
		if len(d.Secret) > 512 {
			return fmt.Errorf("push secret is %d bytes, devices.secret holds 512", len(d.Secret))
		}
		secret = hex.EncodeToString(d.Secret)
	}
	other := formatUIDs(d.OtherUIDs)
	noMuted := 0
	if d.NoMuted {
		noMuted = 1
	}
	sandbox := 0
	if d.AppSandbox {
		sandbox = 1
	}
	_, err := m.db.ExecContext(ctx,
		`DELETE FROM devices WHERE user_id = ? AND token_type = ? AND token = ? AND auth_key_id <> ?`,
		d.UserID, d.TokenType, d.Token, d.AuthKeyID)
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, `
INSERT INTO devices (auth_key_id, user_id, token_type, token, no_muted, locked_period, app_sandbox, secret, other_uids, state)
VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, 0)
ON DUPLICATE KEY UPDATE
  token = VALUES(token),
  no_muted = VALUES(no_muted),
  app_sandbox = VALUES(app_sandbox),
  secret = VALUES(secret),
  other_uids = VALUES(other_uids),
  state = 0`,
		d.AuthKeyID, d.UserID, d.TokenType, d.Token, noMuted, sandbox, secret, other)
	return err
}

func (m *MySQL) Delete(ctx context.Context, userID int64, tokenType int32, token string, otherUserIDs []int64) error {
	ids := append([]int64{userID}, otherUserIDs...)
	placeholders := make([]string, len(ids))
	args := make([]any, 0, 2+len(ids))
	args = append(args, tokenType, token)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := `DELETE FROM devices WHERE token_type = ? AND token = ? AND user_id IN (` + strings.Join(placeholders, ",") + `)`
	_, err := m.db.ExecContext(ctx, q, args...)
	return err
}

func (m *MySQL) ListByUser(ctx context.Context, userID int64) ([]Device, error) {
	rows, err := m.db.QueryContext(ctx, `
SELECT auth_key_id, user_id, token_type, token, no_muted, app_sandbox, secret, other_uids
FROM devices WHERE user_id = ? AND state = 0`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var (
			d       Device
			noMuted int
			sandbox int
			secret  string
			other   string
		)
		if err := rows.Scan(&d.AuthKeyID, &d.UserID, &d.TokenType, &d.Token, &noMuted, &sandbox, &secret, &other); err != nil {
			return nil, err
		}
		d.NoMuted = noMuted != 0
		d.AppSandbox = sandbox != 0
		d.OtherUIDs = parseUIDs(other)
		if secret != "" {
			b, err := hex.DecodeString(secret)
			if err != nil {
				return nil, fmt.Errorf("device secret: %w", err)
			}
			d.Secret = b
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (m *MySQL) NotifySettings(ctx context.Context, userID int64, peerType int32, peerID int64) (NotifySettings, error) {
	var s NotifySettings
	err := m.db.QueryRowContext(ctx, `
SELECT show_previews, silent, mute_until, sound
FROM user_notify_settings
WHERE user_id = ? AND peer_type = ? AND peer_id = ? AND deleted = 0
LIMIT 1`, userID, peerType, peerID).Scan(&s.ShowPreviews, &s.Silent, &s.MuteUntil, &s.Sound)
	if err == sql.ErrNoRows {
		return NotifySettings{}, nil
	}
	if err != nil {
		return NotifySettings{}, err
	}
	s.Found = true
	return s, nil
}

func (m *MySQL) UserName(ctx context.Context, id int64) (string, error) {
	var first, last string
	err := m.db.QueryRowContext(ctx, `SELECT first_name, last_name FROM users WHERE id = ?`, id).Scan(&first, &last)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return joinName(first, last), nil
}

func (m *MySQL) ChatTitle(ctx context.Context, id int64) (string, error) {
	var title string
	err := m.db.QueryRowContext(ctx, `SELECT title FROM chats WHERE id = ?`, id).Scan(&title)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return title, err
}

func formatUIDs(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	s := strings.Join(parts, ",")
	if len(s) > 1024 {
		s = s[:1024]
		if i := strings.LastIndex(s, ","); i > 0 {
			s = s[:i]
		}
	}
	return s
}

func parseUIDs(s string) []int64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func joinName(first, last string) string {
	first = strings.TrimSpace(first)
	last = strings.TrimSpace(last)
	switch {
	case first != "" && last != "":
		return first + " " + last
	case first != "":
		return first
	default:
		return last
	}
}
