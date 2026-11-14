package accesshash

import (
	"context"
	"sync/atomic"

	"github.com/teamgram/proto/mtproto"
)

// Issuing reports whether user and channel objects should carry the stored
// access_hash. Off (the default) leaves every response as it was. Log and
// enforce fill the hash so clients can cache the value the gate will check.
func Issuing() bool {
	return Active() != ModeOff
}

type hashLookup func(context.Context, []int64) (map[int64]int64, error)

var (
	userLookup    atomic.Value // hashLookup
	channelLookup atomic.Value // hashLookup
)

// SetUserLookup registers how this process reads users.access_hash.
// The bff process installs it. A nil function clears it. Returned hashes
// of 0 are ignored.
func SetUserLookup(fn func(context.Context, []int64) (map[int64]int64, error)) {
	if fn == nil {
		userLookup.Store(hashLookup(nil))
		return
	}
	userLookup.Store(hashLookup(fn))
}

// SetChannelLookup registers how this process reads channels.access_hash.
func SetChannelLookup(fn func(context.Context, []int64) (map[int64]int64, error)) {
	if fn == nil {
		channelLookup.Store(hashLookup(nil))
		return
	}
	channelLookup.Store(hashLookup(fn))
}

// UserHash is the non-zero access_hash stored for id.
func UserHash(ctx context.Context, id int64) (int64, bool) {
	h := lookupOne(ctx, userLookup, id)
	return h, h != 0
}

// ChannelHash is the non-zero access_hash stored for id.
func ChannelHash(ctx context.Context, id int64) (int64, bool) {
	h := lookupOne(ctx, channelLookup, id)
	return h, h != 0
}

func lookupOne(ctx context.Context, slot atomic.Value, id int64) int64 {
	if id == 0 {
		return 0
	}
	fn, _ := slot.Load().(hashLookup)
	if fn == nil {
		return 0
	}
	got, err := fn(ctx, []int64{id})
	if err != nil {
		return 0
	}
	return got[id]
}

func lookupMany(ctx context.Context, slot atomic.Value, ids []int64) map[int64]int64 {
	fn, _ := slot.Load().(hashLookup)
	if fn == nil || len(ids) == 0 {
		return nil
	}
	got, err := fn(ctx, ids)
	if err != nil {
		return nil
	}
	return got
}

// PaintUsers sets access_hash from the stored row when Issuing is on.
// A hash of 0 is not written. Min is cleared once a real hash is attached,
// so the client caches that hash instead of treating the user as min.
// Off is a no-op.
func PaintUsers(ctx context.Context, users []*mtproto.User) {
	if !Issuing() || len(users) == 0 {
		return
	}
	ids := make([]int64, 0, len(users))
	seen := map[int64]struct{}{}
	for _, u := range users {
		id := u.GetId()
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	got := lookupMany(ctx, userLookup, ids)
	for _, u := range users {
		h := got[u.GetId()]
		if h == 0 {
			continue
		}
		u.AccessHash = mtproto.MakeFlagsInt64(h)
		u.Min = false
	}
}

// PaintUser is PaintUsers for one user.
func PaintUser(ctx context.Context, user *mtproto.User) {
	if user == nil {
		return
	}
	PaintUsers(ctx, []*mtproto.User{user})
}

// PaintChannels sets access_hash from the stored row when Issuing is on.
func PaintChannels(ctx context.Context, chats []*mtproto.Chat) {
	if !Issuing() || len(chats) == 0 {
		return
	}
	ids := make([]int64, 0, len(chats))
	for _, ch := range chats {
		if ch.GetId() != 0 {
			ids = append(ids, ch.GetId())
		}
	}
	got := lookupMany(ctx, channelLookup, ids)
	for _, ch := range chats {
		h := got[ch.GetId()]
		if h == 0 {
			continue
		}
		ch.AccessHash_FLAGINT64 = mtproto.MakeFlagsInt64(h)
	}
}

// PaintChannel is PaintChannels for one chat.
func PaintChannel(ctx context.Context, chat *mtproto.Chat) {
	if chat == nil {
		return
	}
	PaintChannels(ctx, []*mtproto.Chat{chat})
}

// DropZero removes 0 from issued hashes. A row of 0 is not a secret.
func DropZero(hashes []int64) []int64 {
	out := hashes[:0]
	for _, h := range hashes {
		if h != 0 {
			out = append(out, h)
		}
	}
	return out
}
