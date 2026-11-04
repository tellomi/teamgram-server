package discoverprofile

import "context"

// Edge is the viewer's row in the author's contacts.
type Edge struct {
	Contact     bool
	CloseFriend bool
}

// Directory reads discover_subject, users, user_privacies, and user_contacts.
// It does not write. Login does not call it.
type Directory interface {
	Users(ctx context.Context, subs [][]byte) (map[string]int64, error)
	Profiles(ctx context.Context, ids []int64) (map[int64]Profile, error)
	Privacy(ctx context.Context, ids []int64) (map[int64]Privacy, error)
	// Edges reports which authors list the viewer. The map key is the author id.
	Edges(ctx context.Context, viewer int64, authors []int64) (map[int64]Edge, error)
}

// Memory is a Directory for tests.
type Memory struct {
	users    map[string]int64
	profiles map[int64]Profile
	privacy  map[int64]Privacy
	edges    map[int64]map[int64]Edge
	fail     error
}

// NewMemory returns an empty directory.
func NewMemory() *Memory {
	return &Memory{
		users:    map[string]int64{},
		profiles: map[int64]Profile{},
		privacy:  map[int64]Privacy{},
		edges:    map[int64]map[int64]Edge{},
	}
}

// Save records sub → userID.
func (m *Memory) Save(userID int64, sub []byte) {
	m.users[subKey(sub)] = userID
}

// PutProfile stores the account row.
func (m *Memory) PutProfile(id int64, p Profile) {
	m.profiles[id] = p
}

// PutPrivacy stores one author's rules.
func (m *Memory) PutPrivacy(id int64, p Privacy) {
	m.privacy[id] = p
}

// PutEdge records that viewer is in the author's contacts.
func (m *Memory) PutEdge(author, viewer int64, closeFriend bool) {
	if m.edges[author] == nil {
		m.edges[author] = map[int64]Edge{}
	}
	m.edges[author][viewer] = Edge{Contact: true, CloseFriend: closeFriend}
}

// Fail makes the next read return err.
func (m *Memory) Fail(err error) {
	m.fail = err
}

// Users resolves subs.
func (m *Memory) Users(context.Context, [][]byte) (map[string]int64, error) {
	if m.fail != nil {
		return nil, m.fail
	}
	out := map[string]int64{}
	for k, id := range m.users {
		out[k] = id
	}
	return out, nil
}

// Profiles returns account rows.
func (m *Memory) Profiles(context.Context, []int64) (map[int64]Profile, error) {
	if m.fail != nil {
		return nil, m.fail
	}
	out := map[int64]Profile{}
	for id, p := range m.profiles {
		out[id] = p
	}
	return out, nil
}

// Privacy returns stored rules.
func (m *Memory) Privacy(context.Context, []int64) (map[int64]Privacy, error) {
	if m.fail != nil {
		return nil, m.fail
	}
	out := map[int64]Privacy{}
	for id, p := range m.privacy {
		out[id] = p
	}
	return out, nil
}

// Edges reports which authors currently list viewer as a contact.
func (m *Memory) Edges(_ context.Context, viewer int64, authors []int64) (map[int64]Edge, error) {
	if m.fail != nil {
		return nil, m.fail
	}
	out := map[int64]Edge{}
	for _, author := range authors {
		if edge, ok := m.edges[author][viewer]; ok && edge.Contact {
			out[author] = edge
		}
	}
	return out, nil
}
