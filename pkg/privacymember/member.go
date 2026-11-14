// Package privacymember answers privacyValueAllowChatParticipants and
// privacyValueDisallowChatParticipants from the shared MySQL tables.
// Basic groups are chat_participants. Supergroups and channels are
// channel_participants. A missing database means membership is unknown:
// an allow-chat rule does not grant and a disallow-chat rule does not deny.
package privacymember

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
)

func init() {
	mtproto.PrivacyChatMember = Lookup
}

var (
	mu sync.RWMutex
	db *sql.DB
)

// UseDB installs the process MySQL pool. The user service already has one.
func UseDB(conn *sql.DB) {
	if conn == nil {
		return
	}
	mu.Lock()
	db = conn
	mu.Unlock()
}

// Lookup reports whether userID is in any of chats.
func Lookup(ctx context.Context, userID int64, chats []int64) (member bool, known bool) {
	conn := database()
	if conn == nil || userID == 0 || len(chats) == 0 {
		return false, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	basic, channels := mtproto.SplitChatAndChannelIdList(chats)
	known = true
	if len(basic) > 0 {
		n, err := count(ctx, conn, "chat_participants", "chat_id", userID, basic)
		if err != nil {
			known = false
		} else if n > 0 {
			return true, true
		}
	}
	if len(channels) > 0 {
		n, err := count(ctx, conn, "channel_participants", "channel_id", userID, channels)
		if err != nil {
			known = false
		} else if n > 0 {
			return true, true
		}
	}
	return false, known
}

func database() *sql.DB {
	mu.RLock()
	conn := db
	mu.RUnlock()
	if conn != nil {
		return conn
	}
	dsn := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN"))
	if dsn == "" {
		return nil
	}
	opened, err := mysqltls.Open(dsn)
	if err != nil {
		return nil
	}
	mu.Lock()
	if db == nil {
		db = opened
	}
	conn = db
	mu.Unlock()
	return conn
}

func count(ctx context.Context, conn *sql.DB, table, column string, userID int64, ids []int64) (int, error) {
	var b strings.Builder
	b.WriteString("SELECT COUNT(*) FROM ")
	b.WriteString(table)
	b.WriteString(" WHERE user_id=? AND state=0 AND ")
	b.WriteString(column)
	b.WriteString(" IN (")
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('?')
		args = append(args, id)
	}
	b.WriteByte(')')
	var n int
	err := conn.QueryRowContext(ctx, b.String(), args...).Scan(&n)
	return n, err
}
