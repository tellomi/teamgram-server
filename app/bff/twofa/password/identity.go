package password

import (
	"context"

	"github.com/teamgram/teamgram-server/pkg/verify"
)

// identityReader is implemented by the memory and MySQL stores.
// It reads users.email. It does not write it.
type identityReader interface {
	IdentityEmail(ctx context.Context, userID int64) (string, error)
	IdentityOwner(ctx context.Context, email string) (int64, error)
}

// IdentityEmail is the normalized users.email for userID.
// An empty string means the account has no email identity (a phone account,
// or a memory store with no seeded row).
func IdentityEmail(ctx context.Context, userID int64) (string, error) {
	return Default().identityEmail(ctx, userID)
}

// IdentityOwner is the user id whose users.email is email, or 0.
func IdentityOwner(ctx context.Context, email string) (int64, error) {
	return Default().identityOwner(ctx, email)
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
		return "", nil
	}
	email, err := r.IdentityEmail(ctx, userID)
	if err != nil || email == "" {
		return "", err
	}
	email = verify.NormalizeEmail(email)
	if !verify.ValidEmail(email) {
		return "", nil
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
		return 0, nil
	}
	return r.IdentityOwner(ctx, email)
}
