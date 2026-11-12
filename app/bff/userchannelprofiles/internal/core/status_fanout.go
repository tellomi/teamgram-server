package core

import "github.com/teamgram/proto/mtproto"

// statusRecipients is who may learn this user's exact online status.
// forward is people this user added. reverse is people who added this user
// but are not already in forward. visible reports this user's
// privacyKeyStatusTimestamp rule toward that viewer: AllowAll includes both
// directions, AllowContacts includes only forward, DisallowAll includes only
// an explicit allow-user exception. Self, empty ids, duplicates, and blocked
// users are left out. Official clients do not receive their own status.
func statusRecipients(self int64, forward, reverse []int64, visible func(int64) bool, blocked func(int64) bool) []int64 {
	seen := make(map[int64]struct{}, len(forward)+len(reverse))
	out := make([]int64, 0, len(forward)+len(reverse))
	consider := func(id int64) {
		if id == 0 || id == self {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		if blocked != nil && blocked(id) {
			return
		}
		if visible != nil && !visible(id) {
			return
		}
		out = append(out, id)
	}
	for _, id := range forward {
		consider(id)
	}
	for _, id := range reverse {
		consider(id)
	}
	return out
}

// deliverUserStatus pushes one updateUserStatus to every remaining recipient.
// Calling it again with the same inputs yields the same recipients.
func deliverUserStatus(self int64, updates *mtproto.Updates, forward, reverse []int64, visible func(int64) bool, blocked func(int64) bool, pushContact func(int64, *mtproto.Updates)) {
	if pushContact == nil {
		return
	}
	for _, id := range statusRecipients(self, forward, reverse, visible, blocked) {
		pushContact(id, updates)
	}
}

// attachForwardContacts lets CheckPrivacy see "my contacts" on a user loaded
// without that edge list. ContactUserId is the person this user added.
func attachForwardContacts(me *mtproto.ImmutableUser, ids []int64) {
	if me == nil {
		return
	}
	rows := make([]*mtproto.ContactData, 0, len(ids))
	for _, id := range ids {
		if id == 0 || id == me.Id() {
			continue
		}
		rows = append(rows, mtproto.MakeTLContactData(&mtproto.ContactData{
			UserId:        me.Id(),
			ContactUserId: id,
		}).To_ContactData())
	}
	me.Contacts = rows
}
