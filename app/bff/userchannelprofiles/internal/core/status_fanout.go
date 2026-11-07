package core

import "github.com/teamgram/proto/mtproto"

// statusRecipients is who may learn this user's online status.
// Self, empty ids, duplicates, and blocked users are left out.
// A user who is not in contacts is not added.
func statusRecipients(self int64, contacts []int64, blocked func(int64) bool) []int64 {
	seen := make(map[int64]struct{}, len(contacts))
	out := make([]int64, 0, len(contacts))
	for _, id := range contacts {
		if id == 0 || id == self {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if blocked != nil && blocked(id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// deliverUserStatus pushes updateUserStatus to the remaining contacts and,
// when this session has an auth key, to the user's other sessions.
// Calling it again with the same contacts yields the same recipients.
func deliverUserStatus(self, authKey int64, updates *mtproto.Updates, contacts []int64, blocked func(int64) bool, pushContact func(int64, *mtproto.Updates), pushOtherSessions func(int64, int64, *mtproto.Updates)) {
	if pushContact != nil {
		for _, id := range statusRecipients(self, contacts, blocked) {
			pushContact(id, updates)
		}
	}
	if authKey != 0 && pushOtherSessions != nil {
		pushOtherSessions(self, authKey, updates)
	}
}
