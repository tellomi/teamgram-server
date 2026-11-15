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

// callerSaved reports whether userID is in the caller's address book.
// contacts.search puts those hits in my_results. CheckContact on the other
// user asks the opposite question: whether they saved the caller. A one-way
// import would then land in results.
func callerSaved(contactIDs map[int64]struct{}, userID int64) bool {
	if userID == 0 || len(contactIDs) == 0 {
		return false
	}
	_, ok := contactIDs[userID]
	return ok
}
