package tellomiai

import (
	"context"
	"database/sql"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	_ "github.com/go-sql-driver/mysql"
)

// MySQL keeps reply rows in tellomi_ai_reply.
type MySQL struct {
	db *sql.DB
}

// OpenMySQL opens the teamgram database. The DSN is not logged.
func OpenMySQL(dsn string) (*MySQL, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errorsEmpty()
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	return &MySQL{db: db}, nil
}

func errorsEmpty() error {
	return errEmptyDSN
}

var errEmptyDSN = errString("empty dsn")

type errString string

func (e errString) Error() string { return string(e) }

// Save inserts one reply. The API key is not a column.
func (m *MySQL) Save(ctx context.Context, rec Record) error {
	if m == nil || m.db == nil {
		return errEmptyDSN
	}
	refused := 0
	if rec.Refused {
		refused = 1
	}
	_, err := m.db.ExecContext(ctx, `
INSERT INTO tellomi_ai_reply (user_id, event_id, model, refused, limited, prompt, reply)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.UserID, clip(rec.EventID, 128), clip(rec.Model, 64), refused, clip(rec.Limited, 16), rec.Prompt, rec.Reply)
	return err
}
