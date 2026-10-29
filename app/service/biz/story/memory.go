package story

import (
	"context"
	"sort"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/teamgram/proto/mtproto"
)

type memoryStore struct {
	mu        sync.Mutex
	nextStory map[Owner]int32
	nextAlbum map[Owner]int32
	stories   map[Owner]map[int32]*Record
	views     map[Owner]map[int32]map[int64]View
	reads     map[int64]map[Owner]*Read
	settings  map[int64]*Settings
	albums    map[Owner]map[int32]*Album
	pinned    map[Owner][]int32
	reports   []*Report
}

func NewMemoryStore() Store {
	return &memoryStore{
		nextStory: map[Owner]int32{},
		nextAlbum: map[Owner]int32{},
		stories:   map[Owner]map[int32]*Record{},
		views:     map[Owner]map[int32]map[int64]View{},
		reads:     map[int64]map[Owner]*Read{},
		settings:  map[int64]*Settings{},
		albums:    map[Owner]map[int32]*Album{},
		pinned:    map[Owner][]int32{},
	}
}

func cloneRecord(r *Record) *Record {
	if r == nil {
		return nil
	}
	cp := *r
	if r.Item != nil {
		cp.Item = proto.Clone(r.Item).(*mtproto.StoryItem)
		cp.Item.SanitizeEntities()
	}
	return &cp
}

func cloneAlbum(a *Album) *Album {
	if a == nil {
		return nil
	}
	cp := *a
	cp.StoryIDs = append([]int32(nil), a.StoryIDs...)
	return &cp
}

func (s *memoryStore) bucket(owner Owner) map[int32]*Record {
	if s.stories[owner] == nil {
		s.stories[owner] = map[int32]*Record{}
	}
	return s.stories[owner]
}

func (s *memoryStore) Create(ctx context.Context, rec *Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.Item.Id == 0 {
		s.nextStory[rec.Owner]++
		rec.Item.Id = s.nextStory[rec.Owner]
	} else if rec.Item.Id > s.nextStory[rec.Owner] {
		s.nextStory[rec.Owner] = rec.Item.Id
	}
	s.bucket(rec.Owner)[rec.Item.Id] = cloneRecord(rec)
	return nil
}

func (s *memoryStore) Save(ctx context.Context, rec *Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec == nil || rec.Item == nil || rec.Item.Id == 0 {
		return errStoryIDInvalid
	}
	if s.stories[rec.Owner] == nil || s.stories[rec.Owner][rec.Item.Id] == nil {
		return errStoryIDInvalid
	}
	s.stories[rec.Owner][rec.Item.Id] = cloneRecord(rec)
	return nil
}

func (s *memoryStore) Get(ctx context.Context, owner Owner, id int32) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stories[owner] == nil {
		return nil, nil
	}
	return cloneRecord(s.stories[owner][id]), nil
}

func (s *memoryStore) FindByRandom(ctx context.Context, owner Owner, sender, randomID int64) (*Record, error) {
	if randomID == 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range s.stories[owner] {
		if rec.Sender == sender && rec.RandomID == randomID && !rec.Deleted {
			return cloneRecord(rec), nil
		}
	}
	return nil, nil
}

func (s *memoryStore) ListByOwner(ctx context.Context, owner Owner) ([]*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked(owner), nil
}

func (s *memoryStore) listLocked(owner Owner) []*Record {
	out := make([]*Record, 0, len(s.stories[owner]))
	for _, rec := range s.stories[owner] {
		out = append(out, cloneRecord(rec))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id() < out[j].id() })
	return out
}

func (s *memoryStore) ListAll(ctx context.Context) ([]*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Record
	for owner := range s.stories {
		out = append(out, s.listLocked(owner)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Item.Date == out[j].Item.Date {
			return out[i].id() < out[j].id()
		}
		return out[i].Item.Date < out[j].Item.Date
	})
	return out, nil
}

func (s *memoryStore) AddForwards(ctx context.Context, owner Owner, id int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.stories[owner][id]
	if rec == nil {
		return nil
	}
	rec.Forwards++
	return nil
}

func (s *memoryStore) SaveView(ctx context.Context, owner Owner, storyID int32, v View) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.views[owner] == nil {
		s.views[owner] = map[int32]map[int64]View{}
	}
	if s.views[owner][storyID] == nil {
		s.views[owner][storyID] = map[int64]View{}
	}
	s.views[owner][storyID][v.UserID] = v
	return nil
}

func (s *memoryStore) ListViews(ctx context.Context, owner Owner, storyID int32) ([]View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []View
	for _, v := range s.views[owner][storyID] {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date == out[j].Date {
			return out[i].UserID < out[j].UserID
		}
		return out[i].Date > out[j].Date
	})
	return out, nil
}

func (s *memoryStore) HideActiveViews(ctx context.Context, viewer int64, now int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for owner, byStory := range s.views {
		for id, byUser := range byStory {
			rec := s.stories[owner][id]
			if rec == nil || rec.Deleted || rec.Item == nil || rec.Item.ExpireDate <= now {
				continue
			}
			delete(byUser, viewer)
		}
	}
	return nil
}

func (s *memoryStore) GetRead(ctx context.Context, userID int64, owner Owner) (*Read, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rd := s.reads[userID][owner]
	if rd == nil {
		return nil, nil
	}
	cp := *rd
	return &cp, nil
}

func (s *memoryStore) SaveRead(ctx context.Context, rd *Read) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reads[rd.UserID] == nil {
		s.reads[rd.UserID] = map[Owner]*Read{}
	}
	cp := *rd
	s.reads[rd.UserID][rd.Owner] = &cp
	return nil
}

func (s *memoryStore) ListReads(ctx context.Context, userID int64) ([]*Read, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Read
	for _, rd := range s.reads[userID] {
		cp := *rd
		out = append(out, &cp)
	}
	return out, nil
}

func (s *memoryStore) GetSettings(ctx context.Context, userID int64) (*Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.settings[userID]
	if st == nil {
		return &Settings{UserID: userID}, nil
	}
	cp := *st
	return &cp, nil
}

func (s *memoryStore) SaveSettings(ctx context.Context, st *Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *st
	s.settings[st.UserID] = &cp
	return nil
}

func (s *memoryStore) CreateAlbum(ctx context.Context, a *Album) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.ID == 0 {
		s.nextAlbum[a.Owner]++
		a.ID = s.nextAlbum[a.Owner]
	}
	if s.albums[a.Owner] == nil {
		s.albums[a.Owner] = map[int32]*Album{}
	}
	if a.Order == 0 {
		a.Order = int32(len(s.albums[a.Owner]) + 1)
	}
	s.albums[a.Owner][a.ID] = cloneAlbum(a)
	return nil
}

func (s *memoryStore) SaveAlbum(ctx context.Context, a *Album) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.albums[a.Owner] == nil || s.albums[a.Owner][a.ID] == nil {
		return errAlbumInvalid
	}
	s.albums[a.Owner][a.ID] = cloneAlbum(a)
	return nil
}

func (s *memoryStore) GetAlbum(ctx context.Context, owner Owner, id int32) (*Album, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneAlbum(s.albums[owner][id]), nil
}

func (s *memoryStore) ListAlbums(ctx context.Context, owner Owner) ([]*Album, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Album, 0, len(s.albums[owner]))
	for _, a := range s.albums[owner] {
		out = append(out, cloneAlbum(a))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order == out[j].Order {
			return out[i].ID < out[j].ID
		}
		return out[i].Order < out[j].Order
	})
	return out, nil
}

func (s *memoryStore) DeleteAlbum(ctx context.Context, owner Owner, id int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.albums[owner], id)
	return nil
}

func (s *memoryStore) SetPinnedTop(ctx context.Context, owner Owner, ids []int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pinned[owner] = append([]int32(nil), ids...)
	return nil
}

func (s *memoryStore) PinnedTop(ctx context.Context, owner Owner) ([]int32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int32(nil), s.pinned[owner]...), nil
}

func (s *memoryStore) SaveReport(ctx context.Context, r *Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *r
	cp.StoryIDs = append([]int32(nil), r.StoryIDs...)
	s.reports = append(s.reports, &cp)
	return nil
}
