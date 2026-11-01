package dao

import "context"

// MemSaved is an in-process phone book for tests.
type MemSaved struct {
	Matched      map[int64][]SavedPhone
	Unregistered map[int64][]SavedPhone
}

func (m *MemSaved) List(_ context.Context, userID int64) ([]SavedPhone, error) {
	out := append([]SavedPhone{}, m.Matched[userID]...)
	return append(out, m.Unregistered[userID]...), nil
}

func (m *MemSaved) UnregisteredCount(_ context.Context, userID int64) (int32, error) {
	return int32(len(m.Unregistered[userID])), nil
}

func (m *MemSaved) ResetUnregistered(_ context.Context, userID int64) error {
	delete(m.Unregistered, userID)
	return nil
}
