package book

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

// collectionStore is the durable copy of star gift collection titles.
// The in-memory maps stay the source the handlers read. Save replaces one
// peer's rows so a restart can load the same order and the next id.
type collectionStore interface {
	Load(ctx context.Context) (map[string][]giftCollection, map[string]int32, error)
	Save(ctx context.Context, key string, rows []giftCollection, next int32) error
}

// SetGiftAdmin installs the channel permission check used by collection
// mutations. Nil rejects every channel with CHAT_ADMIN_REQUIRED. A user may
// manage only their own profile. A basic group is PEER_ID_INVALID.
// https://core.telegram.org/method/payments.createStarGiftCollection
func (b *Book) SetGiftAdmin(fn func(self, channelID int64) error) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.giftAdmin = fn
	b.mu.Unlock()
}

// OpenCollections attaches MySQL when dsn or TEAMGRAM_MYSQL_DSN is set.
// An empty DSN leaves titles in memory. A DSN that cannot be opened is
// returned so the caller can log it; the book keeps serving from memory.
func (b *Book) OpenCollections(dsn string) error {
	if b == nil {
		return nil
	}
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		dsn = env
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil
	}
	st, err := openCollectionMySQL(dsn)
	if err != nil {
		return err
	}
	return b.useCollectionStore(st)
}

func (b *Book) useCollectionStore(st collectionStore) error {
	if st == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, next, err := st.Load(ctx)
	if err != nil {
		return err
	}
	if rows == nil {
		rows = map[string][]giftCollection{}
	}
	if next == nil {
		next = map[string]int32{}
	}
	b.mu.Lock()
	b.gifts.cols = rows
	b.gifts.nextID = next
	b.cols = st
	b.mu.Unlock()
	return nil
}

func (b *Book) giftAdminFn() func(int64, int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.giftAdmin
}

// mayManageCollection is the Telegram rule for collection mutations.
// The owner of a user profile, or a channel admin with can_post_messages,
// may create, rename, reorder, and delete. Anyone may still read the list.
func (b *Book) mayManageCollection(self int64, ref peerRef) error {
	switch ref.kind {
	case "user":
		if ref.id != self {
			return fail(CodePeerInvalid)
		}
		return nil
	case "channel":
		fn := b.giftAdminFn()
		if fn == nil {
			return mtproto.ErrChatAdminRequired
		}
		return fn(self, ref.id)
	default:
		return fail(CodePeerInvalid)
	}
}

func snapshotCols(g *giftState, key string) ([]giftCollection, int32) {
	return append([]giftCollection(nil), g.cols[key]...), g.nextID[key]
}

func (b *Book) saveCollectionsLocked(key string, prev []giftCollection, prevNext int32) error {
	if b.cols == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows := append([]giftCollection(nil), b.gifts.cols[key]...)
	err := b.cols.Save(ctx, key, rows, b.gifts.nextID[key])
	if err != nil {
		if len(prev) == 0 {
			delete(b.gifts.cols, key)
		} else {
			b.gifts.cols[key] = prev
		}
		if prevNext == 0 {
			delete(b.gifts.nextID, key)
		} else {
			b.gifts.nextID[key] = prevNext
		}
		logx.Errorf("star gift collections: save failed: %v", err)
		return mtproto.ErrInternalServerError
	}
	return nil
}
