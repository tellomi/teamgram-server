package dao

import (
	"context"
	"database/sql"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	"github.com/teamgram/proto/mtproto"

	_ "github.com/go-sql-driver/mysql"
)

// Saved contacts.
//
// contacts.getSaved exports the phone book during a takeout session:
// https://core.telegram.org/method/contacts.getSaved
// https://core.telegram.org/api/takeout
//
// contacts.resetSaved removes contacts that have no Telegram account:
// https://core.telegram.org/method/contacts.resetSaved
// Those rows are unregistered_contacts (imported = 0). Matched contacts stay
// in user_contacts.

// SavedPhone is one savedPhoneContact row.
type SavedPhone struct {
	Phone     string
	FirstName string
	LastName  string
	Date      int32
}

// SavedBook reads and clears the server-side phone book.
type SavedBook interface {
	List(ctx context.Context, userID int64) ([]SavedPhone, error)
	UnregisteredCount(ctx context.Context, userID int64) (int32, error)
	ResetUnregistered(ctx context.Context, userID int64) error
	// DeletePhones removes phone-book rows for contacts.deleteByPhones.
	// Registered rows are deleted by the user service; this drops the
	// unregistered copy so saved_count stays honest.
	DeletePhones(ctx context.Context, userID int64, phones []string) error
}

// SQLSaved reads user_contacts and unregistered_contacts.
type SQLSaved struct {
	db *sql.DB
}

// OpenSaved opens the teamgram MySQL DSN. An empty DSN returns nil.
func OpenSaved(dsn string) *SQLSaved {
	if dsn == "" {
		return nil
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return nil
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	return &SQLSaved{db: db}
}

// List returns matched contacts and phone-book rows that never matched a user.
func (s *SQLSaved) List(ctx context.Context, userID int64) ([]SavedPhone, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	matched, err := s.query(ctx, `
		SELECT contact_phone, contact_first_name, contact_last_name, date2
		FROM user_contacts
		WHERE owner_user_id = ? AND is_deleted = 0 AND contact_phone <> ''
		ORDER BY contact_user_id`, userID)
	if err != nil {
		return nil, err
	}
	plain, err := s.query(ctx, `
		SELECT phone, import_first_name, import_last_name, UNIX_TIMESTAMP(created_at)
		FROM unregistered_contacts
		WHERE importer_user_id = ? AND imported = 0
		ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	return append(matched, plain...), nil
}

// UnregisteredCount is contacts.contacts.saved_count: rows resetSaved deletes.
func (s *SQLSaved) UnregisteredCount(ctx context.Context, userID int64) (int32, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	var n int32
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM unregistered_contacts
		WHERE importer_user_id = ? AND imported = 0`, userID).Scan(&n)
	return n, err
}

// ResetUnregistered marks phone-book rows that have no Telegram account as imported.
func (s *SQLSaved) ResetUnregistered(ctx context.Context, userID int64) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE unregistered_contacts SET imported = 1
		WHERE importer_user_id = ? AND imported = 0`, userID)
	return err
}

// DeletePhones marks the given unregistered phone-book rows as imported.
func (s *SQLSaved) DeletePhones(ctx context.Context, userID int64, phones []string) error {
	if s == nil || s.db == nil {
		return nil
	}
	keys := phoneKeys(phones)
	if len(keys) == 0 {
		return nil
	}
	args := make([]any, 0, len(keys)+1)
	args = append(args, userID)
	holders := make([]string, len(keys))
	for i, phone := range keys {
		holders[i] = "?"
		args = append(args, phone)
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE unregistered_contacts SET imported = 1
		WHERE importer_user_id = ? AND imported = 0 AND phone IN (`+strings.Join(holders, ",")+`)`, args...)
	return err
}

func (s *SQLSaved) query(ctx context.Context, q string, userID int64) ([]SavedPhone, error) {
	rows, err := s.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SavedPhone
	for rows.Next() {
		var phone, first, last string
		var date int64
		if err := rows.Scan(&phone, &first, &last, &date); err != nil {
			return nil, err
		}
		if date < 0 {
			date = 0
		}
		if date > int64(^uint32(0)>>1) {
			date = int64(^uint32(0) >> 1)
		}
		out = append(out, SavedPhone{
			Phone: phone, FirstName: first, LastName: last, Date: int32(date),
		})
	}
	return out, rows.Err()
}

// ToSavedContact builds the MTProto savedPhoneContact.
func ToSavedContact(row SavedPhone) *mtproto.SavedContact {
	return mtproto.MakeTLSavedPhoneContact(&mtproto.SavedContact{
		Phone:     row.Phone,
		FirstName: row.FirstName,
		LastName:  row.LastName,
		Date:      row.Date,
	}).To_SavedContact()
}
