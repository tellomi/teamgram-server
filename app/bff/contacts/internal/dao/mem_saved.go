package dao

import (
	"context"
	"strings"
)

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

func (m *MemSaved) DeletePhones(_ context.Context, userID int64, phones []string) error {
	if m == nil {
		return nil
	}
	want := map[string]bool{}
	for _, phone := range phoneKeys(phones) {
		want[phone] = true
	}
	if m.Unregistered == nil {
		m.Unregistered = map[int64][]SavedPhone{}
	}
	if m.Matched == nil {
		m.Matched = map[int64][]SavedPhone{}
	}
	m.Unregistered[userID] = dropPhones(m.Unregistered[userID], want)
	m.Matched[userID] = dropPhones(m.Matched[userID], want)
	return nil
}

func dropPhones(rows []SavedPhone, want map[string]bool) []SavedPhone {
	if len(want) == 0 {
		return rows
	}
	kept := rows[:0]
	for _, row := range rows {
		if want[row.Phone] || want[digitsOnly(row.Phone)] {
			continue
		}
		kept = append(kept, row)
	}
	return kept
}

func phoneKeys(phones []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, phone := range phones {
		phone = strings.TrimSpace(phone)
		if phone == "" {
			continue
		}
		if !seen[phone] {
			seen[phone] = true
			out = append(out, phone)
		}
		if d := digitsOnly(phone); d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
