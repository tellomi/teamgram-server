package linkpreview

import (
	"context"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
)

// Memory is an in-memory public catalog used by tests and by a process that
// has no MySQL DSN. It only returns records that were inserted as public.
type Memory struct {
	mu       sync.Mutex
	peers    map[string]*storedPeer
	channels map[int64]*storedPeer
	invites  map[string]*storedInvite
	messages map[int64]map[int32]*mtproto.Message
	stories  map[storyKey]*storedStory
	packs    map[string]*storedPack
	photos   map[int64]string
	docs     map[int64]string
}

type storyKey struct {
	kind  int8
	owner int64
	id    int32
}

func NewMemory() *Memory {
	return &Memory{
		peers:    map[string]*storedPeer{},
		channels: map[int64]*storedPeer{},
		invites:  map[string]*storedInvite{},
		messages: map[int64]map[int32]*mtproto.Message{},
		stories:  map[storyKey]*storedStory{},
		packs:    map[string]*storedPack{},
		photos:   map[int64]string{},
		docs:     map[int64]string{},
	}
}

func (m *Memory) PutPeer(p storedPeer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := p
	m.peers[strings.ToLower(p.Username)] = &cp
	if p.Kind == KindChannel || p.Kind == KindGroup {
		ch := cp
		m.channels[p.ID] = &ch
	}
}

func (m *Memory) PutInvite(hash string, inv storedInvite) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := inv
	m.invites[hash] = &cp
}

func (m *Memory) PutMessage(channelID int64, msg *mtproto.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.messages[channelID] == nil {
		m.messages[channelID] = map[int32]*mtproto.Message{}
	}
	m.messages[channelID][msg.Id] = msg
}

func (m *Memory) PutStory(ownerKind int8, ownerID int64, item *mtproto.StoryItem, deleted bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stories[storyKey{kind: ownerKind, owner: ownerID, id: item.Id}] = &storedStory{Item: item, Deleted: deleted}
}

func (m *Memory) PutPack(p storedPack) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := p
	m.packs[packKey(p.ShortName, p.Emoji)] = &cp
}

func (m *Memory) PutPhoto(id int64, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.photos[id] = path
}

func (m *Memory) PutDocument(id int64, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.docs[id] = path
}

func packKey(short string, emoji bool) string {
	if emoji {
		return "e:" + strings.ToLower(short)
	}
	return "s:" + strings.ToLower(short)
}

func (m *Memory) PeerByUsername(_ context.Context, username string) (*storedPeer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.peers[strings.ToLower(username)]
	if p == nil || p.Username == "" {
		return nil, errNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *Memory) PeerByChannelID(_ context.Context, id int64) (*storedPeer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.channels[id]
	if p == nil || p.Username == "" {
		return nil, errNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *Memory) Invite(_ context.Context, hash string) (*storedInvite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv := m.invites[hash]
	if inv == nil {
		return nil, errNotFound
	}
	cp := *inv
	return &cp, nil
}

func (m *Memory) Message(_ context.Context, channelID int64, id int32) (*storedMsg, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg := m.messages[channelID][id]
	if msg == nil {
		return nil, errNotFound
	}
	return &storedMsg{Msg: msg}, nil
}

func (m *Memory) Latest(_ context.Context, channelID int64, limit int) ([]storedMsg, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.messages[channelID]
	ids := make([]int32, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}
	sortDesc(ids)
	if limit <= 0 || limit > len(ids) {
		limit = len(ids)
	}
	out := make([]storedMsg, 0, limit)
	for _, id := range ids[:limit] {
		out = append(out, storedMsg{Msg: all[id]})
	}
	return out, nil
}

func sortDesc(ids []int32) {
	for i := 1; i < len(ids); i++ {
		j := i
		for j > 0 && ids[j] > ids[j-1] {
			ids[j], ids[j-1] = ids[j-1], ids[j]
			j--
		}
	}
}

func (m *Memory) Story(_ context.Context, ownerKind int8, ownerID int64, id int32) (*storedStory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.stories[storyKey{kind: ownerKind, owner: ownerID, id: id}]
	if st == nil {
		return nil, errNotFound
	}
	return st, nil
}

func (m *Memory) Pack(_ context.Context, short string, emoji bool) (*storedPack, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.packs[packKey(short, emoji)]
	if p == nil {
		return nil, errNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *Memory) FilePath(_ context.Context, photoID, documentID int64) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if photoID != 0 {
		if p := m.photos[photoID]; p != "" {
			return p, nil
		}
	}
	if documentID != 0 {
		if p := m.docs[documentID]; p != "" {
			return p, nil
		}
	}
	return "", errNotFound
}
