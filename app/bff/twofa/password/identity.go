package password

import (
	"context"
	"errors"

	"github.com/teamgram/teamgram-server/pkg/verify"
)

// ErrIdentityUnconfirmed means users.email could not be read as either a
// confirmed phone account (NULL email) or a valid email identity.
// Callers must not treat it as a phone account.
var ErrIdentityUnconfirmed = errors.New("login identity is not confirmed")

// identityReader is implemented by the memory and MySQL stores.
// It reads users.email. It does not write it.
type identityReader interface {
	IdentityEmail(ctx context.Context, userID int64) (string, error)
	IdentityOwner(ctx context.Context, email string) (int64, error)
}

// IdentityEmail is the normalized users.email for userID.
// An empty string with a nil error is a confirmed phone account.
// ErrIdentityUnconfirmed means the row is missing, the value is not an
// address, or the user table cannot be read.
func IdentityEmail(ctx context.Context, userID int64) (string, error) {
	return Default().identityEmail(ctx, userID)
}

// IdentityOwner is the user id whose users.email is email, or 0.
func IdentityOwner(ctx context.Context, email string) (int64, error) {
	return Default().identityOwner(ctx, email)
}

// SeedRecovery stores a cloud-password recovery address on the current store.
// It does not write users.email or the Redis login-email copy.
func SeedRecovery(userID int64, email string, hasPassword bool) error {
	s := Default()
	if s == nil || s.store == nil || userID == 0 {
		return nil
	}
	rec, err := s.load(context.Background(), userID)
	if err != nil {
		return err
	}
	rec.UserID = userID
	rec.Email = email
	rec.HasPassword = hasPassword
	rec.Exists = true
	return s.store.Save(context.Background(), rec)
}

// SeedPhone marks userID as a confirmed phone account (users.email NULL)
// on the in-memory ledger. A MySQL store ignores this call.
func SeedPhone(userID int64) {
	m, ok := Default().store.(*memoryStore)
	if !ok {
		return
	}
	m.setPhone(userID)
}

// SeedIdentity records users.email on the in-memory store used by tests.
// A MySQL store reads the users table and ignores this call.
func SeedIdentity(userID int64, email string) {
	m, ok := Default().store.(*memoryStore)
	if !ok {
		return
	}
	m.setIdentity(userID, email)
}

func (s *Service) identityEmail(ctx context.Context, userID int64) (string, error) {
	if s == nil || s.store == nil || userID == 0 {
		return "", nil
	}
	r, ok := s.store.(identityReader)
	if !ok {
		return "", ErrIdentityUnconfirmed
	}
	email, err := r.IdentityEmail(ctx, userID)
	if err != nil {
		return "", err
	}
	email = verify.NormalizeEmail(email)
	if email == "" {
		return "", nil
	}
	if !verify.ValidEmail(email) {
		return "", ErrIdentityUnconfirmed
	}
	return email, nil
}

func (s *Service) identityOwner(ctx context.Context, email string) (int64, error) {
	if s == nil || s.store == nil {
		return 0, nil
	}
	email = verify.NormalizeEmail(email)
	if !verify.ValidEmail(email) {
		return 0, nil
	}
	r, ok := s.store.(identityReader)
	if !ok {
		return 0, ErrIdentityUnconfirmed
	}
	return r.IdentityOwner(ctx, email)
}
