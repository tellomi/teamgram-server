package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// DelCache failure must survive the real handler and the story drain.
// BoolFalse with a nil error used to delete the durable dirty row.
func TestStoriesMaxCacheFailureKeepsDirtyRow(t *testing.T) {
	dsn, raw := openStoriesMaxSchema(t)
	ctx := context.Background()
	if _, err := raw.Exec(`CREATE TABLE users (id BIGINT NOT NULL PRIMARY KEY, stories_max_id INT NOT NULL DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`
CREATE TABLE story_advert_dirty (
  user_id BIGINT NOT NULL,
  stories_max_id INT NOT NULL,
  updated_at INT NOT NULL,
  PRIMARY KEY (user_id)
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO users (id, stories_max_id) VALUES (7, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO story_advert_dirty (user_id, stories_max_id, updated_at) VALUES (7, 4, 1)`); err != nil {
		t.Fatal(err)
	}
	svcCtx := &svc.ServiceContext{Dao: &dao.Dao{
		CachedConn: sqlc.NewConnWithCache(sqlx.NewMySQL(&sqlx.Config{DSN: dsn}), failDelCache{}),
	}}
	userCore := New(ctx, svcCtx)
	stored, err := userCore.UserSetStoriesMaxId(&user.TLUserSetStoriesMaxId{UserId: 7, Id: 4})
	if err == nil || mtproto.FromBool(stored) {
		t.Fatalf("handler stored=%v err=%v", stored, err)
	}
	if err = story.RejectUnstoredAdvert(stored, err); err == nil {
		t.Fatal("cache failure became a successful store")
	}

	stories, err := story.OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	stories.SetHooks(story.Hooks{SetUserMax: func(ctx context.Context, userID int64, maxID int32) error {
		got, callErr := userCore.UserSetStoriesMaxId(&user.TLUserSetStoriesMaxId{UserId: userID, Id: maxID})
		return story.RejectUnstoredAdvert(got, callErr)
	}})
	stories.ReleaseExpired(ctx)
	var left int
	if err = raw.QueryRow(`SELECT COUNT(*) FROM story_advert_dirty WHERE user_id=7`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Fatalf("dirty rows %d", left)
	}
}

func openStoriesMaxSchema(t *testing.T) (string, *sql.DB) {
	t.Helper()
	admin, err := sql.Open("mysql", "storyadvert:storyadvert@tcp(127.0.0.1:3306)/")
	if err != nil {
		t.Skipf("mysql open: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = admin.PingContext(ctx); err != nil {
		_ = admin.Close()
		t.Skipf("mysql not available: %v", err)
	}
	name := fmt.Sprintf("story_advert_%d_max", os.Getpid())
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE `" + name + "`")
		_ = admin.Close()
	})
	dsn := "storyadvert:storyadvert@tcp(127.0.0.1:3306)/" + name + "?charset=utf8mb4&parseTime=true"
	raw, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return dsn, raw
}

type failDelCache struct{}

func (failDelCache) Del(...string) error { return errors.New("redis down") }
func (failDelCache) DelCtx(context.Context, ...string) error {
	return errors.New("redis down")
}
func (failDelCache) Get(string, any) error                          { return nil }
func (failDelCache) GetCtx(context.Context, string, any) error      { return nil }
func (failDelCache) IsNotFound(error) bool                          { return false }
func (failDelCache) Set(string, any) error                          { return nil }
func (failDelCache) SetCtx(context.Context, string, any) error      { return nil }
func (failDelCache) SetWithExpire(string, any, time.Duration) error { return nil }
func (failDelCache) SetWithExpireCtx(context.Context, string, any, time.Duration) error {
	return nil
}
func (failDelCache) Take(any, string, func(any) error) error { return nil }
func (failDelCache) TakeCtx(context.Context, any, string, func(any) error) error {
	return nil
}
func (failDelCache) TakeWithExpire(any, string, func(any, time.Duration) error) error {
	return nil
}
func (failDelCache) TakeWithExpireCtx(context.Context, any, string, func(any, time.Duration) error) error {
	return nil
}
func (failDelCache) Takes(func(...string) (map[string]any, error), func(string, string) (any, error), ...string) error {
	return nil
}
func (failDelCache) TakesCtx(context.Context, func(...string) (map[string]any, error), func(string, string) (any, error), ...string) error {
	return nil
}
func (failDelCache) TakesWithExpire(func(time.Duration, ...string) (map[string]any, error), func(string, string) (any, error), ...string) error {
	return nil
}
func (failDelCache) TakesWithExpireCtx(context.Context, func(time.Duration, ...string) (map[string]any, error), func(string, string) (any, error), ...string) error {
	return nil
}
