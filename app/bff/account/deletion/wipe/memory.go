package wipe

import (
	"context"
	"sort"
	"strconv"
	"sync"
)

// Mem is an in-memory store for tests. Production uses SQL.
type Mem struct {
	mu       sync.Mutex
	boxes    []memBox
	dialogs  map[dialogKey]bool // true = live
	groups   map[int64]*memGroup
	channels map[int64]*memGroup
	profile  map[int64]memProfile
	contacts []memContact
	settings map[int64]int
	objects  map[string][]Object // kind:id -> objects
	queue    []GCItem
	pts      map[int64]int32
	ptsRows  []memPts
	nextGC   int64
	refs     map[string]int
	shared   map[string]bool
	exempt   map[int64]bool
}

type memBox struct {
	User int64
	Peer Peer
	Box  Box
	Gone bool
}

type dialogKey struct {
	User int64
	Type int32
	Peer int64
}

type memGroup struct {
	ID          int64
	Creator     int64
	Version     int32
	Deactivated bool
	Members     []Member
}

type memProfile struct {
	Username string
	Name     string
	About    string
	Birthday string
	Photo    int64
	Photos   []int64
}

type memContact struct {
	Owner   int64
	Contact int64
	Deleted bool
}

type memPts struct {
	User int64
	Pts  int32
	N    int32
	JSON []byte
}

func NewMem() *Mem {
	return &Mem{
		dialogs:  map[dialogKey]bool{},
		groups:   map[int64]*memGroup{},
		channels: map[int64]*memGroup{},
		profile:  map[int64]memProfile{},
		settings: map[int64]int{},
		objects:  map[string][]Object{},
		pts:      map[int64]int32{},
		refs:     map[string]int{},
		shared:   map[string]bool{},
		exempt:   map[int64]bool{},
	}
}

func objKey(kind string, id int64) string {
	return kind + ":" + strconv.FormatInt(id, 10)
}

func (m *Mem) AddBox(user int64, peer Peer, box Box) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.boxes = append(m.boxes, memBox{User: user, Peer: peer, Box: box})
	m.dialogs[dialogKey{user, peer.Type, peer.ID}] = true
}

func (m *Mem) AddGroup(channel bool, g memGroup) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := g
	cp.Members = append([]Member(nil), g.Members...)
	if channel {
		m.channels[g.ID] = &cp
		return
	}
	m.groups[g.ID] = &cp
}

func (m *Mem) SetProfile(user int64, p memProfile) {
	m.mu.Lock()
	m.profile[user] = p
	m.mu.Unlock()
}

func (m *Mem) AddContact(owner, contact int64) {
	m.mu.Lock()
	m.contacts = append(m.contacts, memContact{Owner: owner, Contact: contact})
	m.mu.Unlock()
}

func (m *Mem) SetSettings(user int64, n int) {
	m.mu.Lock()
	m.settings[user] = n
	m.mu.Unlock()
}

func (m *Mem) SetObjects(kind string, id int64, objs []Object) {
	m.mu.Lock()
	m.objects[objKey(kind, id)] = append([]Object(nil), objs...)
	m.mu.Unlock()
}

func (m *Mem) SetRefs(kind string, id int64, n int) {
	m.mu.Lock()
	m.refs[objKey(kind, id)] = n
	m.mu.Unlock()
}

// SetShared marks an object key as still held by another media_blob_refs row.
func (m *Mem) SetShared(class, path string) {
	m.mu.Lock()
	m.shared[class+"/"+path] = true
	m.mu.Unlock()
}

func (m *Mem) SetExempt(documentID int64) {
	m.mu.Lock()
	m.exempt[documentID] = true
	m.mu.Unlock()
}

func (m *Mem) LiveBoxes(user int64, peer Peer) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, b := range m.boxes {
		if !b.Gone && b.User == user && b.Peer == peer {
			n++
		}
	}
	return n
}

func (m *Mem) DialogLive(user int64, peer Peer) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dialogs[dialogKey{user, peer.Type, peer.ID}]
}

func (m *Mem) Group(id int64) memGroup {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.groups[id]
	if g == nil {
		g = m.channels[id]
	}
	if g == nil {
		return memGroup{}
	}
	cp := *g
	cp.Members = append([]Member(nil), g.Members...)
	return cp
}

func (m *Mem) Profile(user int64) memProfile {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.profile[user]
}

func (m *Mem) ContactLive(owner, contact int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.contacts {
		if c.Owner == owner && c.Contact == contact && !c.Deleted {
			return true
		}
	}
	return false
}

func (m *Mem) Settings(user int64) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings[user]
}

func (m *Mem) Queue() []GCItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]GCItem(nil), m.queue...)
}

func (m *Mem) PtsRows(user int64) []memPts {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []memPts
	for _, row := range m.ptsRows {
		if row.User == user {
			out = append(out, row)
		}
	}
	return out
}

func (m *Mem) PrivatePeers(_ context.Context, userID int64) ([]Peer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[Peer]bool{}
	var out []Peer
	for _, b := range m.boxes {
		if b.Gone || b.User != userID || (b.Peer.Type != 1 && b.Peer.Type != 2) {
			continue
		}
		if !seen[b.Peer] {
			seen[b.Peer] = true
			out = append(out, b.Peer)
		}
	}
	for key, live := range m.dialogs {
		if !live || key.User != userID || (key.Type != 1 && key.Type != 2) {
			continue
		}
		p := Peer{Type: key.Type, ID: key.Peer}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *Mem) Boxes(_ context.Context, userID int64, peer Peer, after int32, limit int) ([]Box, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Box
	for _, b := range m.boxes {
		if b.Gone || b.User != userID || b.Peer != peer || b.Box.ID <= after {
			continue
		}
		out = append(out, b.Box)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Mem) CommitDelete(_ context.Context, userID int64, ids []int32, pts, ptsCount, _ int32, updateJSON []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[int32]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for i := range m.boxes {
		if m.boxes[i].User == userID && want[m.boxes[i].Box.ID] {
			m.boxes[i].Gone = true
		}
	}
	if pts > 0 {
		m.ptsRows = append(m.ptsRows, memPts{User: userID, Pts: pts, N: ptsCount, JSON: append([]byte(nil), updateJSON...)})
	}
	return nil
}

func (m *Mem) DeleteDialog(_ context.Context, userID int64, peer Peer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.dialogs, dialogKey{userID, peer.Type, peer.ID})
	return nil
}

func (m *Mem) AllocPts(_ context.Context, userID int64, n int32) (int32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pts[userID] += n
	return m.pts[userID], nil
}

func (m *Mem) BasicGroups(_ context.Context, userID int64) ([]int64, error) {
	return m.memberships(userID, false), nil
}

func (m *Mem) memberships(userID int64, channel bool) []int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.groups
	if channel {
		src = m.channels
	}
	var ids []int64
	for id, g := range src {
		for _, mem := range g.Members {
			if mem.UserID == userID && mem.State == 0 {
				ids = append(ids, id)
			}
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (m *Mem) LeaveBasic(ctx context.Context, userID, chatID, now int64) (LeftNotice, error) {
	return m.leave(userID, chatID, now, false)
}

func (m *Mem) LeaveChannels(_ context.Context, userID, now int64) ([]LeftNotice, error) {
	ids := m.memberships(userID, true)
	var out []LeftNotice
	for _, id := range ids {
		notice, err := m.leave(userID, id, now, true)
		if err != nil {
			return nil, err
		}
		if notice.ChatID != 0 {
			out = append(out, notice)
		}
	}
	return out, nil
}

func (m *Mem) leave(userID, chatID, now int64, channel bool) (LeftNotice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.groups
	if channel {
		src = m.channels
	}
	g := src[chatID]
	if g == nil {
		return LeftNotice{}, nil
	}
	idx := -1
	for i, mem := range g.Members {
		if mem.UserID == userID && mem.State == 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		return LeftNotice{}, nil
	}
	notice := LeftNotice{ChatID: chatID, Channel: channel}
	me := g.Members[idx]
	if me.Role == 1 || g.Creator == userID {
		to, ok := PickSuccessor(g.Members, userID)
		if !ok {
			g.Deactivated = true
			g.Members[idx].State = 1
			g.Version++
			notice.Deactivated = true
			notice.Version = g.Version
			return notice, nil
		}
		for i := range g.Members {
			if g.Members[i].UserID == userID && g.Members[i].State == 0 {
				g.Members[i].Role = 2
			}
			if g.Members[i].UserID == to && g.Members[i].State == 0 {
				g.Members[i].Role = 1
			}
		}
		g.Creator = to
		notice.Successor = to
	}
	g.Members[idx].State = 1
	g.Version++
	notice.Version = g.Version
	for _, mem := range g.Members {
		if mem.State == 0 {
			notice.Members = append(notice.Members, mem.UserID)
		}
	}
	_ = now
	return notice, nil
}

func (m *Mem) ClearProfile(_ context.Context, userID int64, username string) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.profile[userID]
	photos := append([]int64(nil), p.Photos...)
	if p.Photo != 0 {
		photos = append(photos, p.Photo)
	}
	p.Username = ""
	p.Name = ""
	p.About = ""
	p.Birthday = ""
	p.Photo = 0
	p.Photos = nil
	if username != "" {
		p.Username = ""
	}
	m.profile[userID] = p
	return photos, nil
}

func (m *Mem) DeleteContacts(_ context.Context, userID int64) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[int64]bool{}
	var owners []int64
	for i := range m.contacts {
		c := &m.contacts[i]
		if c.Deleted {
			continue
		}
		if c.Owner == userID || c.Contact == userID {
			c.Deleted = true
			if c.Owner != userID && !seen[c.Owner] {
				seen[c.Owner] = true
				owners = append(owners, c.Owner)
			}
		}
	}
	return owners, nil
}

func (m *Mem) DeleteSettings(_ context.Context, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.settings, userID)
	return nil
}

func (m *Mem) PhotoObjects(_ context.Context, photoID int64, avatar bool) ([]Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	got := append([]Object(nil), m.objects[objKey("photo", photoID)]...)
	if avatar {
		got = append(got, m.objects[objKey("avatar", photoID)]...)
	}
	return got, nil
}

func (m *Mem) DocumentObjects(_ context.Context, documentID int64) ([]Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Object(nil), m.objects[objKey("document", documentID)]...), nil
}

func (m *Mem) EncryptedObjects(_ context.Context, fileID int64) ([]Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Object(nil), m.objects[objKey("encrypted", fileID)]...), nil
}

func (m *Mem) Enqueue(_ context.Context, userID int64, objs []Object) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, obj := range objs {
		if obj.Path == "" {
			continue
		}
		dup := false
		for _, q := range m.queue {
			if q.UserID == userID && q.Class == obj.Class && q.Path == obj.Path {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		m.nextGC++
		m.queue = append(m.queue, GCItem{
			ID: m.nextGC, UserID: userID, Kind: obj.Kind, MediaID: obj.ID,
			Class: obj.Class, Path: obj.Path, Status: statusPending,
		})
	}
	return nil
}

func (m *Mem) Due(_ context.Context, status string, limit int) ([]GCItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []GCItem
	for _, item := range m.queue {
		if item.Status == status {
			out = append(out, item)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *Mem) Refs(_ context.Context, item GCItem) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.refs[objKey(item.Kind, item.MediaID)]; ok {
		return n, nil
	}
	return 0, nil
}

func (m *Mem) Shared(_ context.Context, item GCItem) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shared[item.Class+"/"+item.Path], nil
}

func (m *Mem) Detach(_ context.Context, item GCItem) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shared[item.Class+"/"+item.Path] {
		return false, nil
	}
	return true, nil
}

func (m *Mem) Exempt(_ context.Context, item GCItem) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.exempt[item.MediaID], nil
}

func (m *Mem) Mark(_ context.Context, id int64, status string, trashedAt int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.queue {
		if m.queue[i].ID == id {
			m.queue[i].Status = status
			m.queue[i].TrashedAt = trashedAt
		}
	}
	return nil
}
