// Package chatdecor remembers the wallpaper object installed by
// messages.setChatWallPaper. Dialog rows store only the id. users.getFullUser
// loads the object from here, including after a process restart when MySQL
// is configured.
package chatdecor

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
	"google.golang.org/protobuf/proto"
)

const schema = `
CREATE TABLE IF NOT EXISTS chat_wallpapers (
  id bigint NOT NULL,
  wallpaper mediumblob NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

var (
	mu     sync.Mutex
	papers = map[int64]*mtproto.WallPaper{}
	dbOnce sync.Once
	db     *sql.DB
)

// UseDSN opens the wallpaper table. TEAMGRAM_MYSQL_DSN overrides dsn.
// An empty DSN keeps the book in memory for this process.
func UseDSN(dsn string) {
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		dsn = env
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return
	}
	dbOnce.Do(func() {
		opened, err := mysqltls.Open(dsn)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err = opened.PingContext(ctx); err != nil {
			_ = opened.Close()
			return
		}
		if _, err = opened.ExecContext(ctx, schema); err != nil {
			_ = opened.Close()
			return
		}
		db = opened
	})
}

// Remember stores paper so a later getFullUser can return it by id.
func Remember(paper *mtproto.WallPaper) {
	if paper == nil || paper.GetId() == 0 {
		return
	}
	cloned, ok := proto.Clone(paper).(*mtproto.WallPaper)
	if !ok || cloned == nil {
		return
	}
	mu.Lock()
	papers[cloned.GetId()] = cloned
	conn := db
	mu.Unlock()
	if conn == nil {
		return
	}
	raw, err := proto.Marshal(cloned)
	if err != nil || len(raw) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = conn.ExecContext(ctx, "INSERT INTO chat_wallpapers (id, wallpaper) VALUES (?, ?) ON DUPLICATE KEY UPDATE wallpaper = VALUES(wallpaper)", cloned.GetId(), raw)
}

// Get returns the wallpaper installed under id.
func Get(id int64) *mtproto.WallPaper {
	if id == 0 {
		return nil
	}
	mu.Lock()
	paper := papers[id]
	conn := db
	mu.Unlock()
	if paper != nil {
		cloned, _ := proto.Clone(paper).(*mtproto.WallPaper)
		return cloned
	}
	if conn == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var raw []byte
	if err := conn.QueryRowContext(ctx, "SELECT wallpaper FROM chat_wallpapers WHERE id = ?", id).Scan(&raw); err != nil || len(raw) == 0 {
		return nil
	}
	loaded := &mtproto.WallPaper{}
	if err := proto.Unmarshal(raw, loaded); err != nil || loaded.GetId() == 0 {
		return nil
	}
	mu.Lock()
	papers[loaded.GetId()] = loaded
	mu.Unlock()
	cloned, _ := proto.Clone(loaded).(*mtproto.WallPaper)
	return cloned
}

// Plugin is the users.getFullUser wallpaper lookup.
type Plugin struct{}

// GetChatWallpaper implements plugin.WallpaperPlugin.
func (Plugin) GetChatWallpaper(_ context.Context, _ int64, wallpaperID int64) *mtproto.WallPaper {
	return Get(wallpaperID)
}
