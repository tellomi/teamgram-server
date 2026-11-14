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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
)

// queryTimeout bounds one membership or block lookup. A member list must not
// wait on a stuck database once per user.
const queryTimeout = 200 * time.Millisecond

func init() {
	mtproto.PrivacyChatMember = Lookup
	mtproto.PrefetchPrivacy = Prefetch
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
// A request cache hit does not touch the database. A miss is one query per
// table, bounded by queryTimeout, and the answer is stored on ctx.
func Lookup(ctx context.Context, userID int64, chats []int64) (member bool, known bool) {
	if userID == 0 || len(chats) == 0 {
		return false, false
	}
	if member, ok := mtproto.Membership(ctx, userID, chats); ok {
		return member, true
	}
	conn := database()
	if conn == nil {
		return false, false
	}
	qctx, cancel := withQueryTimeout(ctx)
	defer cancel()
	found, err := memberChats(qctx, conn, userID, chats)
	if err != nil {
		return false, false
	}
	mtproto.NoteMembership(ctx, userID, chats, found)
	return len(found) > 0, true
}

// Prefetch loads the viewer's membership in chats and which owners blocked
// the viewer. Both are one query (two when the chat list mixes basic groups
// and channels), shared by every user rendered from ctx.
func Prefetch(ctx context.Context, viewer int64, owners, chats []int64) {
	if viewer == 0 {
		return
	}
	conn := database()
	if conn == nil {
		return
	}
	qctx, cancel := withQueryTimeout(ctx)
	defer cancel()
	if len(chats) > 0 {
		found, err := memberChats(qctx, conn, viewer, chats)
		if err == nil {
			mtproto.NoteMembership(ctx, viewer, chats, found)
		}
	}
	if len(owners) > 0 {
		blocked, err := ownersWhoBlocked(qctx, conn, viewer, owners)
		if err == nil {
			mtproto.NoteBlocked(ctx, owners, blocked)
		}
	}
}

func withQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, queryTimeout)
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

func memberChats(ctx context.Context, conn *sql.DB, userID int64, chats []int64) ([]int64, error) {
	basic, channels := mtproto.SplitChatAndChannelIdList(chats)
	var found []int64
	if len(basic) > 0 {
		ids, err := memberIDs(ctx, conn, "chat_participants", "chat_id", userID, basic)
		if err != nil {
			return nil, err
		}
		found = append(found, ids...)
	}
	if len(channels) > 0 {
		ids, err := memberIDs(ctx, conn, "channel_participants", "channel_id", userID, channels)
		if err != nil {
			return nil, err
		}
		found = append(found, ids...)
	}
	return found, nil
}

func memberIDs(ctx context.Context, conn *sql.DB, table, column string, userID int64, ids []int64) ([]int64, error) {
	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(column)
	b.WriteString(" FROM ")
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
	rows, err := conn.QueryContext(ctx, b.String(), args...)
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

// ownersWhoBlocked returns the subset of owners who blocked viewer.
func ownersWhoBlocked(ctx context.Context, conn *sql.DB, viewer int64, owners []int64) ([]int64, error) {
	var b strings.Builder
	b.WriteString("SELECT user_id FROM user_peer_blocks WHERE peer_type=? AND peer_id=? AND deleted=0 AND user_id IN (")
	args := make([]any, 0, len(owners)+2)
	args = append(args, mtproto.PEER_USER, viewer)
	for i, id := range owners {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('?')
		args = append(args, id)
	}
	b.WriteByte(')')
	rows, err := conn.QueryContext(ctx, b.String(), args...)
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
