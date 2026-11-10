package msgexpiry

import "sync"

// secretSink is the durable copy of KindSecret only. Private chats and
// view-once media are deleted by the history TTL sweeper and the ttl-media
// worker. This sink does not store those kinds.
type secretSink interface {
	Put(Record)
	All() []Record
	MarkSwept(scope int64, id int32)
}

var (
	secretMu   sync.Mutex
	secretRows secretSink
)

// SetSecretSink installs the durable secret-chat log. Nil clears it. Tests
// use a memory sink. The process installs MySQL from secretchat.UseDSN.
func SetSecretSink(s secretSink) {
	secretMu.Lock()
	secretRows = s
	secretMu.Unlock()
}

func currentSecretSink() secretSink {
	secretMu.Lock()
	defer secretMu.Unlock()
	return secretRows
}

func saveSecret(r Record) {
	if r.Kind != KindSecret {
		return
	}
	if s := currentSecretSink(); s != nil {
		s.Put(r)
	}
}

func markSecretSwept(r Record) {
	if s := currentSecretSink(); s != nil {
		s.MarkSwept(r.Scope, r.ID)
	}
}

// memSecret is the in-process stand-in used by tests.
type memSecret struct {
	mu   sync.Mutex
	rows map[secretKey]Record
}

type secretKey struct {
	scope int64
	id    int32
}

func newMemSecret() *memSecret {
	return &memSecret{rows: map[secretKey]Record{}}
}

func (m *memSecret) Put(r Record) {
	if m == nil || r.Kind != KindSecret || r.ID == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := secretKey{r.Scope, r.ID}
	if prev, ok := m.rows[k]; ok && prev.Deleted {
		r.Deleted = true
	}
	m.rows[k] = r
}

func (m *memSecret) All() []Record {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.rows))
	for _, r := range m.rows {
		out = append(out, r)
	}
	return out
}

func (m *memSecret) MarkSwept(scope int64, id int32) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := secretKey{scope, id}
	r, ok := m.rows[k]
	if !ok {
		return
	}
	r.Deleted = true
	m.rows[k] = r
}
