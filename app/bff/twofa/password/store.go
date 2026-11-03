package password

import (
	"context"
	"sync"
	"time"
)

// Record is one user's cloud password. Exists is false when no row is stored.
type Record struct {
	UserID            int64
	HasPassword       bool
	Algo              string
	Salt1             []byte
	Salt2             []byte
	Verifier          []byte
	Hint              string
	Email             string
	EmailUnconfirmed  string
	EmailCode         string
	EmailCodeExpires  int64
	EmailCodeSentAt   int64
	EmailCodeAttempts int
	SecureAlgo        string
	SecureSalt        []byte
	SecureSecret      []byte
	SecureSecretID    int64
	PendingReset      int64
	// PasswordSetAt is unix time when the verifier last changed.
	// Zero means the row predates that column; callers treat it as older
	// than seven days so an omitted password cannot delete the account at once.
	PasswordSetAt int64
	Version       int64
	FailCount     int
	FloodUntil    int64
	NewSalt1      []byte
	NewSalt2      []byte
	NewSecureSalt []byte
	Exists        bool
}

// Challenge is one SRP attempt. Taking it removes it.
type Challenge struct {
	ID        int64
	UserID    int64
	AuthKeyID int64
	Priv      []byte
	Pub       []byte
	Version   int64
	Expires   int64
}

// Store is the password tables. Memory is the default; MySQL is used when a DSN is set.
type Store interface {
	Load(ctx context.Context, userID int64) (Record, error)
	Save(ctx context.Context, rec Record) error
	PutChallenge(ctx context.Context, ch Challenge) error
	TakeChallenge(ctx context.Context, id int64) (Challenge, bool, error)
	PutLogin(ctx context.Context, authKey, userID, at int64) error
	LoginUser(ctx context.Context, authKey int64) (int64, bool, error)
	DeleteLogin(ctx context.Context, authKey int64) error
	DeleteUserLogins(ctx context.Context, userID int64) error
}

type memoryStore struct {
	mu         sync.Mutex
	users      map[int64]Record
	challenges map[int64]Challenge
	logins     map[int64]int64
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		users:      map[int64]Record{},
		challenges: map[int64]Challenge{},
		logins:     map[int64]int64{},
	}
}

func (m *memoryStore) Load(_ context.Context, userID int64) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.users[userID]
	if !ok {
		return Record{UserID: userID}, nil
	}
	return cloneRecord(rec), nil
}

func (m *memoryStore) Save(_ context.Context, rec Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec.Exists = true
	m.users[rec.UserID] = cloneRecord(rec)
	return nil
}

func (m *memoryStore) PutChallenge(_ context.Context, ch Challenge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	dropStaleChallenges(m.challenges, ch)
	m.challenges[ch.ID] = ch
	return nil
}

// dropStaleChallenges deletes this user's expired rows. account.getPassword
// mints a new row every call; a live srp_id stays valid until it expires or
// the password changes. The delete is limited to one user so it stays cheap.
func dropStaleChallenges(challenges map[int64]Challenge, ch Challenge) {
	cutoff := ch.Expires - int64(challengeTTL/time.Second)
	for id, old := range challenges {
		if old.UserID != ch.UserID || id == ch.ID {
			continue
		}
		if old.Expires > 0 && old.Expires <= cutoff {
			delete(challenges, id)
		}
	}
}

func (m *memoryStore) TakeChallenge(_ context.Context, id int64) (Challenge, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.challenges[id]
	if !ok {
		return Challenge{}, false, nil
	}
	delete(m.challenges, id)
	return ch, true, nil
}

func (m *memoryStore) PutLogin(_ context.Context, authKey, userID, _ int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logins[authKey] = userID
	return nil
}

func (m *memoryStore) LoginUser(_ context.Context, authKey int64) (int64, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.logins[authKey]
	return id, ok, nil
}

func (m *memoryStore) DeleteLogin(_ context.Context, authKey int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.logins, authKey)
	return nil
}

func (m *memoryStore) DeleteUserLogins(_ context.Context, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, id := range m.logins {
		if id == userID {
			delete(m.logins, key)
		}
	}
	return nil
}

func cloneRecord(rec Record) Record {
	rec.Salt1 = append([]byte(nil), rec.Salt1...)
	rec.Salt2 = append([]byte(nil), rec.Salt2...)
	rec.Verifier = append([]byte(nil), rec.Verifier...)
	rec.SecureSalt = append([]byte(nil), rec.SecureSalt...)
	rec.SecureSecret = append([]byte(nil), rec.SecureSecret...)
	rec.NewSalt1 = append([]byte(nil), rec.NewSalt1...)
	rec.NewSalt2 = append([]byte(nil), rec.NewSalt2...)
	rec.NewSecureSalt = append([]byte(nil), rec.NewSecureSalt...)
	return rec
}
