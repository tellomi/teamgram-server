package core

import "strings"

// contactNoteMatch reports whether q matches a contact's address-book name.
// The account name is searched separately. A multi-word query matches the
// note "名 姓" and the same words in either order.
func contactNoteMatch(q, first, last string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return false
	}
	full := strings.ToLower(strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last)))
	packed := strings.ReplaceAll(full, " ", "")
	qPacked := strings.ReplaceAll(q, " ", "")
	if strings.Contains(full, q) || (qPacked != "" && strings.Contains(packed, qPacked)) {
		return true
	}
	parts := strings.Fields(q)
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		part = strings.ToLower(part)
		if !strings.Contains(full, part) && !strings.Contains(packed, part) {
			return false
		}
	}
	return true
}
