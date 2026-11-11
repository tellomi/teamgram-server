package push

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	_ "github.com/go-sql-driver/mysql"
)

// Open connects to the teamgram database that holds `devices` and
// `user_notify_settings`. sql.Open does not dial; the first query does.
func Open(dsn string) (*MySQL, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("empty mysql dsn")
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)
	return &MySQL{db: db}, nil
}

// OpenConfigured opens the device store. TEAMGRAM_MYSQL_DSN overrides dsn.
// An empty DSN returns a nil store and a nil error so logout can still
// succeed when push storage is not configured.
func OpenConfigured(dsn string) (*MySQL, error) {
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		dsn = env
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, nil
	}
	return Open(dsn)
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
	return m.upsertRegistration(ctx, d, secret, other, noMuted, sandbox)
}

func (m *MySQL) DeleteAuthKey(ctx context.Context, userID, authKeyID int64) error {
	if userID == 0 {
		return nil
	}
	if authKeyID == 0 {
		_, err := m.db.ExecContext(ctx, `DELETE FROM devices WHERE user_id = ?`, userID)
		return err
	}
	_, err := m.db.ExecContext(ctx, `DELETE FROM devices WHERE user_id = ? AND auth_key_id = ?`, userID, authKeyID)
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

// listByUserQuery loads push registrations whose login is still live.
// auth.logOut and the reset-authorization methods set auth_users.deleted.
// A device row left behind by a client that never called account.unregisterDevice
// must not be pushed. auths is only the language source: initConnection
// rewrites auths.lang_code, and devices.lang_code is the registration snapshot.
// Newest registrations are first so a phone that just registered is not queued
// behind older sessions of the same account.
const listByUserQuery = `
SELECT d.auth_key_id, d.user_id, d.token_type, d.token, d.no_muted, d.app_sandbox, d.secret, d.other_uids, d.locked_period,
       COALESCE(NULLIF(a.lang_code, ''), d.lang_code),
       COALESCE(a.system_lang_code, '')
FROM devices d
JOIN auth_users au ON au.auth_key_id = d.auth_key_id AND au.user_id = d.user_id AND au.deleted = 0
LEFT JOIN auths a ON a.auth_key_id = d.auth_key_id AND a.deleted = 0
WHERE d.user_id = ? AND d.state = 0
ORDER BY d.updated_at DESC, d.id DESC`

func (m *MySQL) ListByUser(ctx context.Context, userID int64) ([]Device, error) {
	rows, err := m.db.QueryContext(ctx, listByUserQuery, userID)
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
		if err := rows.Scan(&d.AuthKeyID, &d.UserID, &d.TokenType, &d.Token, &noMuted, &sandbox, &secret, &other, &d.LockedPeriod, &d.LangCode, &d.SystemLangCode); err != nil {
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

func (m *MySQL) SetLockedPeriod(ctx context.Context, userID, authKeyID int64, period int32) error {
	_, err := m.db.ExecContext(ctx, `
UPDATE devices SET locked_period = ? WHERE user_id = ? AND auth_key_id = ? AND state = 0`,
		period, userID, authKeyID)
	return err
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
