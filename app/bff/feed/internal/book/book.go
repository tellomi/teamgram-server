// Package book stores feed read cursors and interest selections.
// There is no channel index, so message pages are empty. The interest
// catalog and the read cursor are real.
package book

import (
	"encoding/binary"
	"hash/fnv"
	"sync"

	"github.com/teamgram/proto/mtproto"
)

// Category is one interest the client can select.
type Category struct {
	ID    int32
	Title string
	Emoji string
}

// Catalog is the interest list feed.getInterestCategories returns.
var Catalog = []Category{
	{1, "News", "📰"},
	{2, "Technology", "💻"},
	{3, "Sports", "⚽"},
	{4, "Entertainment", "🎬"},
	{5, "Science", "🔬"},
	{6, "Business", "💼"},
	{7, "Travel", "✈️"},
	{8, "Music", "🎵"},
}

type user struct {
	selected []int32
	read     map[int32]*mtproto.FeedPosition
}

// Book is the process-local feed state.
type Book struct {
	mu    sync.Mutex
	users map[int64]*user
}

var (
	once   sync.Once
	shared *Book
)

// Shared is the process-wide book.
func Shared() *Book {
	once.Do(func() { shared = New() })
	return shared
}

// New returns an empty book.
func New() *Book {
	return &Book{users: map[int64]*user{}}
}

func (b *Book) box(self int64) *user {
	u := b.users[self]
	if u == nil {
		u = &user{read: map[int32]*mtproto.FeedPosition{}}
		b.users[self] = u
	}
	if u.read == nil {
		u.read = map[int32]*mtproto.FeedPosition{}
	}
	return u
}

// Interests returns the catalog. A matching hash returns not-modified.
func (b *Book) Interests(self, hash int64) (*mtproto.Feed_InterestCategories, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	selected := append([]int32(nil), b.box(self).selected...)
	sum := interestHash(selected)
	if hash != 0 && hash == sum {
		return mtproto.MakeFeedInterestCategoriesNotModified(), nil
	}
	rows := make([]*mtproto.Feed_InterestCategory, 0, len(Catalog))
	for _, row := range Catalog {
		rows = append(rows, mtproto.MakeFeedInterestCategory(row.ID, row.Title, row.Emoji))
	}
	return mtproto.MakeFeedInterestCategories(rows, selected), nil
}

// SetInterests replaces the caller's selection. Unknown ids are rejected.
func (b *Book) SetInterests(self int64, ids []int32) (*mtproto.Bool, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	seen := map[int32]bool{}
	selected := make([]int32, 0, len(ids))
	for _, id := range ids {
		if !known(id) || seen[id] {
			if !known(id) {
				return nil, mtproto.ErrInterestCategoryInvalid
			}
			continue
		}
		seen[id] = true
		selected = append(selected, id)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.box(self).selected = selected
	return mtproto.BoolTrue, nil
}

// Feed returns an empty message page. The read cursor is included when set.
func (b *Book) Feed(self int64, in *mtproto.TLFeedGetFeed) (*mtproto.Feed_FeedMessages, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetFilterId() < 0 {
		return nil, mtproto.ErrFilterIdInvalid
	}
	if in.GetLimit() < 0 || in.GetLimit() > 100 {
		return nil, mtproto.ErrLimitInvalid
	}
	if err := checkPos(in.GetOffsetPosition(), false); err != nil {
		return nil, err
	}
	if err := checkPos(in.GetMaxPosition(), false); err != nil {
		return nil, err
	}
	if err := checkPos(in.GetMinPosition(), false); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var read *mtproto.FeedPosition
	if in.GetOffsetToMaxRead() {
		read = b.box(self).read[in.GetFilterId()]
	} else if stored := b.box(self).read[in.GetFilterId()]; stored != nil {
		read = stored
	}
	sum := feedHash(in.GetFilterId(), read)
	if in.GetHash() != 0 && in.GetHash() == sum {
		return mtproto.MakeFeedMessagesNotModified(), nil
	}
	return mtproto.MakeFeedMessages(nil, nil, read, nil, nil, nil), nil
}

// Read stores the read cursor. The returned updates carry seq 0.
func (b *Book) Read(self int64, filter int32, pos *mtproto.FeedPosition) (*mtproto.Updates, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if filter < 0 {
		return nil, mtproto.ErrFilterIdInvalid
	}
	if err := checkPos(pos, true); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.box(self).read[filter] = mtproto.MakeFeedPosition(pos.GetDate(), pos.GetId(), pos.GetPeer())
	return mtproto.MakeEmptyUpdates(), nil
}

// ForYou returns the interest picker until the caller selects categories.
func (b *Book) ForYou(self int64, in *mtproto.TLFeedGetForYou) (*mtproto.Feed_ForYouFeed, error) {
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetLimit() < 0 || in.GetLimit() > 100 {
		return nil, mtproto.ErrLimitInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.box(self).selected) == 0 {
		return mtproto.MakeFeedForYou([]*mtproto.FeedItem{mtproto.MakeFeedInterestItem()}, nil, nil), nil
	}
	_ = in.GetQuery()
	return mtproto.MakeFeedForYou(nil, nil, nil), nil
}

func known(id int32) bool {
	for _, row := range Catalog {
		if row.ID == id {
			return true
		}
	}
	return false
}

func checkPos(pos *mtproto.FeedPosition, required bool) error {
	if pos == nil {
		if required {
			return mtproto.ErrPeerIdInvalid
		}
		return nil
	}
	peer := pos.GetPeer()
	if peer == nil || (peer.GetUserId() == 0 && peer.GetChatId() == 0 && peer.GetChannelId() == 0) {
		return mtproto.ErrPeerIdInvalid
	}
	if pos.GetId() <= 0 {
		return mtproto.ErrMessageIdInvalid
	}
	return nil
}

func interestHash(ids []int32) int64 {
	h := fnv.New64a()
	for _, id := range ids {
		_ = binary.Write(h, binary.LittleEndian, id)
	}
	sum := int64(h.Sum64())
	if sum == 0 {
		return 1
	}
	return sum
}

func feedHash(filter int32, read *mtproto.FeedPosition) int64 {
	h := fnv.New64a()
	_ = binary.Write(h, binary.LittleEndian, filter)
	if read != nil {
		_ = binary.Write(h, binary.LittleEndian, read.GetDate())
		_ = binary.Write(h, binary.LittleEndian, read.GetId())
		if peer := read.GetPeer(); peer != nil {
			_ = binary.Write(h, binary.LittleEndian, peer.GetUserId())
			_ = binary.Write(h, binary.LittleEndian, peer.GetChannelId())
		}
	}
	sum := int64(h.Sum64())
	if sum == 0 {
		return 1
	}
	return sum
}
