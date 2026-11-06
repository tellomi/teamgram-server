package svc

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// The production hook is applyStoryMax. BoolFalse with a nil error is the
// user-service response that used to be treated as a successful cache drop.
func TestApplyStoryMaxFalseDoesNotClearDirty(t *testing.T) {
	dsn, raw := openChainSchema(t)
	ctx := context.Background()
	stories, err := story.OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(`
CREATE TABLE story_advert_dirty (
  user_id BIGINT NOT NULL,
  stories_max_id INT NOT NULL,
  updated_at INT NOT NULL,
  PRIMARY KEY (user_id)
)`); err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(`INSERT INTO story_advert_dirty (user_id, stories_max_id, updated_at) VALUES (7, 4, 1)`); err != nil {
		t.Fatal(err)
	}
	stories.SetHooks(story.Hooks{SetUserMax: func(ctx context.Context, userID int64, maxID int32) error {
		return applyStoryMax(ctx, falseMaxUsers{}, nil, userID, maxID)
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

type falseMaxUsers struct{}

func (falseMaxUsers) UserSetStoriesMaxId(context.Context, *user.TLUserSetStoriesMaxId) (*mtproto.Bool, error) {
	return mtproto.BoolFalse, nil
}

func (falseMaxUsers) UserGetImmutableUserV2(context.Context, *user.TLUserGetImmutableUserV2) (*mtproto.ImmutableUser, error) {
	return nil, nil
}

func (falseMaxUsers) UserGetMutableUsers(context.Context, *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error) {
	return nil, nil
}

func openChainSchema(t *testing.T) (string, *sql.DB) {
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
	name := fmt.Sprintf("story_advert_%d_chain", os.Getpid())
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE `" + name + "`")
		_ = admin.Close()
	})
	dsn := "storyadvert:storyadvert@tcp(127.0.0.1:3306)/" + name + "?charset=utf8mb4&parseTime=true&multiStatements=true"
	raw, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return dsn, raw
}
