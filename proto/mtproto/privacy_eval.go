package mtproto

// PrivacySubject is the user a privacy rule list is applied to.
// Member reports chat membership. The bool is whether they are in any of
// the chats. known is false when membership cannot be checked: an allow-chat
// rule then does not grant, and a disallow-chat rule does not deny.
type PrivacySubject struct {
	UserID      int64
	Contact     bool
	CloseFriend bool
	Premium     bool
	Bot         bool
	Member      func(chats []int64) (member bool, known bool)
}

// EvaluatePrivacy applies one user's privacy rules to subject.
//
// Official clients store one base rule (everybody, contacts, or nobody)
// plus exceptions. Exceptions win over the base, and a "never" exception
// wins over an "always" exception when both match:
//
//  1. disallow users
//  2. allow users
//  3. disallow chat participants, when membership is known
//  4. allow chat participants, when membership is known
//  5. disallow bots / allow bots
//  6. allow close friends
//  7. allow premium
//  8. disallow contacts, when the base is everybody
//  9. the base rule
//
// An empty list does not allow. Callers that have no stored rules apply the
// key's default list first.
func EvaluatePrivacy(rules []*PrivacyRule, sub PrivacySubject) bool {
	if len(rules) == 0 {
		return false
	}
	var (
		base              = RULE_TYPE_INVALID
		allowUsers        []int64
		disallowUsers     []int64
		allowChats        []int64
		disallowChats     []int64
		allowCloseFriends bool
		allowPremium      bool
		allowBots         bool
		disallowBots      bool
		disallowContacts  bool
	)
	for _, r := range rules {
		if r == nil {
			continue
		}
		switch r.PredicateName {
		case Predicate_privacyValueAllowAll:
			base = ALLOW_ALL
		case Predicate_privacyValueAllowContacts:
			base = ALLOW_CONTACTS
		case Predicate_privacyValueDisallowAll:
			base = DISALLOW_ALL
		case Predicate_privacyValueDisallowContacts:
			disallowContacts = true
		case Predicate_privacyValueAllowUsers:
			allowUsers = append(allowUsers, r.Users...)
		case Predicate_privacyValueDisallowUsers:
			disallowUsers = append(disallowUsers, r.Users...)
		case Predicate_privacyValueAllowChatParticipants:
			allowChats = append(allowChats, r.Chats...)
		case Predicate_privacyValueDisallowChatParticipants:
			disallowChats = append(disallowChats, r.Chats...)
		case Predicate_privacyValueAllowCloseFriends:
			allowCloseFriends = true
		case Predicate_privacyValueAllowPremium:
			allowPremium = true
		case Predicate_privacyValueAllowBots:
			allowBots = true
		case Predicate_privacyValueDisallowBots:
			disallowBots = true
		}
	}
	if base == RULE_TYPE_INVALID && disallowContacts && !hasAllowBase(rules) {
		base = DISALLOW_CONTACTS
	}

	if containsID(disallowUsers, sub.UserID) {
		return false
	}
	if containsID(allowUsers, sub.UserID) {
		return true
	}
	if len(disallowChats) > 0 {
		if member, known := chatMember(sub, disallowChats); known && member {
			return false
		}
	}
	if len(allowChats) > 0 {
		if member, known := chatMember(sub, allowChats); known && member {
			return true
		}
	}
	if sub.Bot && disallowBots {
		return false
	}
	if sub.Bot && allowBots {
		return true
	}
	if allowCloseFriends && sub.CloseFriend {
		return true
	}
	if allowPremium && sub.Premium {
		return true
	}
	if disallowContacts && sub.Contact && base == ALLOW_ALL {
		return false
	}

	switch base {
	case ALLOW_ALL:
		return true
	case ALLOW_CONTACTS:
		return sub.Contact
	case DISALLOW_CONTACTS:
		return !sub.Contact
	case DISALLOW_ALL:
		return false
	default:
		return false
	}
}

func hasAllowBase(rules []*PrivacyRule) bool {
	for _, r := range rules {
		if r == nil {
			continue
		}
		switch r.PredicateName {
		case Predicate_privacyValueAllowAll, Predicate_privacyValueAllowContacts, Predicate_privacyValueDisallowAll:
			return true
		}
	}
	return false
}

func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func chatMember(sub PrivacySubject, chats []int64) (bool, bool) {
	if sub.Member == nil || len(chats) == 0 {
		return false, false
	}
	return sub.Member(chats)
}
