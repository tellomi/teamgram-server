package discoverrel

import "context"

// Directory maps subs to users and reads the author's live contacts.
type Directory interface {
	Save(ctx context.Context, userID int64, sub []byte) error
	Users(ctx context.Context, subs [][]byte) (map[string]int64, error)
	// Edges reports which authors list viewer in their contacts. The map key
	// is the author user id.
	Edges(ctx context.Context, viewer int64, authors []int64) (map[int64]Edge, error)
}

// Memory is a Directory for tests.
type Memory struct {
	users map[string]int64
	// edges[owner][peer]
	edges map[int64]map[int64]Edge
}

// NewMemory returns an empty directory.
func NewMemory() *Memory {
	return &Memory{users: map[string]int64{}, edges: map[int64]map[int64]Edge{}}
}

// Save records sub → userID.
func (m *Memory) Save(_ context.Context, userID int64, sub []byte) error {
	if userID <= 0 || len(sub) != 16 {
		return errBad
	}
	m.users[subKey(sub)] = userID
	return nil
}

// PutEdge records that viewer is in the author's contacts.
func (m *Memory) PutEdge(author, viewer int64, closeFriend bool) {
	if m.edges[author] == nil {
		m.edges[author] = map[int64]Edge{}
	}
	m.edges[author][viewer] = Edge{Contact: true, CloseFriend: closeFriend}
}

// Users resolves subs.
func (m *Memory) Users(_ context.Context, subs [][]byte) (map[string]int64, error) {
	out := map[string]int64{}
	for _, sub := range subs {
		key := subKey(sub)
		if id, ok := m.users[key]; ok {
			out[key] = id
		}
	}
	return out, nil
}

// Edges reports which authors currently list viewer as a contact.
// The row is the author's contact list (Stories "my contacts" / close friends),
// not the viewer's.
func (m *Memory) Edges(_ context.Context, viewer int64, authors []int64) (map[int64]Edge, error) {
	out := map[int64]Edge{}
	for _, author := range authors {
		if edge, ok := m.edges[author][viewer]; ok && edge.Contact {
			out[author] = edge
		}
	}
	return out, nil
}
