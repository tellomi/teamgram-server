package deletion

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/account/deletion/wipe"
	"github.com/teamgram/teamgram-server/app/bff/phone/secretchat"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
	"github.com/zeromicro/go-zero/core/logx"
)

// ErrNotConfigured is returned when ChannelMySQL was not opened.
var ErrNotConfigured = errNotConfigured{}

type errNotConfigured struct{}

func (errNotConfigured) Error() string { return "account wipe is not configured" }

var (
	runMu  sync.Mutex
	engine *wipe.Engine
	gcDB   *wipe.SQL
	mover  wipe.Mover
	endpt  string
	ptsFn  func(context.Context, int64, int32) (int32, error)
)

// Configured reports whether the shared purge can wipe history and profile.
func Configured() bool {
	runMu.Lock()
	defer runMu.Unlock()
	return engine != nil
}

// SetPts installs the idgen pts allocator. Nil keeps the MySQL MAX(pts) fallback.
func SetPts(fn func(context.Context, int64, int32) (int32, error)) {
	runMu.Lock()
	ptsFn = fn
	if engine != nil {
		engine.Pts = fn
	}
	runMu.Unlock()
}

// ConfigureWipe opens the teamgram schema. An empty DSN leaves the wipe off.
func ConfigureWipe(dsn string, notify sync_client.SyncClient, cache wipe.Cache) error {
	if dsn == "" {
		return ErrNotConfigured
	}
	shards := 1
	if n, err := strconv.Atoi(os.Getenv("TELLOMI_MESSAGE_SHARDING")); err == nil && n > 1 {
		shards = n
	}
	st, err := wipe.OpenSQL(dsn, shards)
	if err != nil {
		return err
	}
	eng := &wipe.Engine{
		Store:  st,
		Notify: syncPush{c: notify},
		Cache:  cache,
		Channels: func(ctx context.Context, userID int64) error {
			if !channel.MySQLOpen() {
				return nil
			}
			return channel.Shared().LeaveDeletedAccount(ctx, userID)
		},
		Secrets: discardSecrets,
		Pts:     ptsFn,
	}
	runMu.Lock()
	engine = eng
	gcDB = st
	runMu.Unlock()
	if c, err := objectstore.Open(); err != nil {
		logx.Errorf("account gc: object storage: %v", err)
	} else if c != nil {
		mover = c
		endpt = c.Endpoint()
	}
	if endpt == "" {
		endpt = os.Getenv("TELLOMI_S3_ENDPOINT")
	}
	return nil
}

// Apply runs the Telegram wipe. It fails closed when MySQL is configured and
// a step errors, so the caller can retry without marking the user deleted.
func Apply(ctx context.Context, userID int64, phone, username string) error {
	runMu.Lock()
	eng := engine
	runMu.Unlock()
	if eng == nil {
		return ErrNotConfigured
	}
	_, err := eng.Run(ctx, wipe.Request{UserID: userID, Phone: phone, Username: username})
	return err
}

// StartGC runs the object-storage cleaner until ctx is cancelled.
// TELLOMI_ACCOUNT_GC_DRY_RUN defaults to true.
func StartGC(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = time.Minute
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		runGC(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				runGC(ctx)
			}
		}
	}()
}

func runGC(ctx context.Context) {
	runMu.Lock()
	st := gcDB
	mv := mover
	ep := endpt
	runMu.Unlock()
	if st == nil {
		return
	}
	if err := st.PrepareChannelMedia(ctx); err != nil {
		logx.Errorf("account gc: channel media scan: %v", err)
	}
	cfg := wipe.GCFromEnv()
	cfg.Endpoint = ep
	rep, err := wipe.RunGC(ctx, st, mv, cfg, time.Now())
	if err != nil {
		logx.Errorf("account gc: %v", err)
		return
	}
	logx.Infof("%s", rep.String())
}

func discardSecrets(ctx context.Context, userID int64) ([]wipe.SecretEvent, []int64, error) {
	events, files := secretchat.Default().DiscardAll(userID, int32(time.Now().Unix()))
	out := make([]wipe.SecretEvent, 0, len(events))
	for _, ev := range events {
		out = append(out, wipe.SecretEvent{UserID: ev.UserID, Update: ev.Update})
	}
	_ = ctx
	return out, files, nil
}

type syncPush struct {
	c sync_client.SyncClient
}

func (s syncPush) Push(ctx context.Context, userID int64, updates []*mtproto.Update) error {
	if s.c == nil || userID == 0 || len(updates) == 0 {
		return nil
	}
	_, err := s.c.SyncPushUpdates(ctx, &syncpb.TLSyncPushUpdates{
		UserId:  userID,
		Updates: mtproto.MakeUpdatesByUpdates(updates...),
	})
	return err
}
