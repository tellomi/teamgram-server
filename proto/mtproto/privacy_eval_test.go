package mtproto

import "testing"

func rule(name string, users, chats []int64) *PrivacyRule {
	return &PrivacyRule{PredicateName: name, Users: users, Chats: chats}
}

func memberOf(chats ...int64) func([]int64) (bool, bool) {
	set := map[int64]struct{}{}
	for _, id := range chats {
		set[id] = struct{}{}
	}
	return func(asked []int64) (bool, bool) {
		for _, id := range asked {
			if _, ok := set[id]; ok {
				return true, true
			}
		}
		return false, true
	}
}

func TestEvaluatePrivacyBaseAndUserExceptions(t *testing.T) {
	nobody := []*PrivacyRule{rule(Predicate_privacyValueDisallowAll, nil, nil)}
	if EvaluatePrivacy(nobody, PrivacySubject{UserID: 3, Contact: true}) {
		t.Fatal("nobody allows a contact")
	}
	everybody := []*PrivacyRule{rule(Predicate_privacyValueAllowAll, nil, nil)}
	if !EvaluatePrivacy(everybody, PrivacySubject{UserID: 3}) {
		t.Fatal("everybody denies a stranger")
	}
	except := []*PrivacyRule{
		rule(Predicate_privacyValueDisallowUsers, []int64{3}, nil),
		rule(Predicate_privacyValueAllowAll, nil, nil),
	}
	if EvaluatePrivacy(except, PrivacySubject{UserID: 3, Contact: true}) {
		t.Fatal("disallow user must deny under everybody")
	}
	if !EvaluatePrivacy(except, PrivacySubject{UserID: 4}) {
		t.Fatal("everybody still allows someone who is not listed")
	}

	contacts := []*PrivacyRule{
		rule(Predicate_privacyValueAllowUsers, []int64{9}, nil),
		rule(Predicate_privacyValueDisallowUsers, []int64{3}, nil),
		rule(Predicate_privacyValueAllowContacts, nil, nil),
	}
	if !EvaluatePrivacy(contacts, PrivacySubject{UserID: 9}) {
		t.Fatal("allow user must grant a non-contact")
	}
	if EvaluatePrivacy(contacts, PrivacySubject{UserID: 3, Contact: true}) {
		t.Fatal("disallow user must deny a contact")
	}
	if !EvaluatePrivacy(contacts, PrivacySubject{UserID: 4, Contact: true}) {
		t.Fatal("contacts base must allow a contact")
	}
	if EvaluatePrivacy(contacts, PrivacySubject{UserID: 4}) {
		t.Fatal("contacts base must deny a stranger")
	}
	// Never wins when the same user is on both lists.
	both := []*PrivacyRule{
		rule(Predicate_privacyValueAllowUsers, []int64{3}, nil),
		rule(Predicate_privacyValueDisallowUsers, []int64{3}, nil),
		rule(Predicate_privacyValueAllowAll, nil, nil),
	}
	if EvaluatePrivacy(both, PrivacySubject{UserID: 3}) {
		t.Fatal("disallow user wins over allow user")
	}
}

func TestEvaluatePrivacyCloseFriendsPremiumBots(t *testing.T) {
	rules := []*PrivacyRule{
		rule(Predicate_privacyValueAllowCloseFriends, nil, nil),
		rule(Predicate_privacyValueAllowPremium, nil, nil),
		rule(Predicate_privacyValueDisallowAll, nil, nil),
	}
	if !EvaluatePrivacy(rules, PrivacySubject{UserID: 3, CloseFriend: true}) {
		t.Fatal("close friend")
	}
	if !EvaluatePrivacy(rules, PrivacySubject{UserID: 3, Premium: true}) {
		t.Fatal("premium")
	}
	if EvaluatePrivacy(rules, PrivacySubject{UserID: 3}) {
		t.Fatal("stranger under nobody")
	}
	blocked := []*PrivacyRule{
		rule(Predicate_privacyValueDisallowUsers, []int64{3}, nil),
		rule(Predicate_privacyValueAllowCloseFriends, nil, nil),
		rule(Predicate_privacyValueDisallowAll, nil, nil),
	}
	if EvaluatePrivacy(blocked, PrivacySubject{UserID: 3, CloseFriend: true, Premium: true}) {
		t.Fatal("never share wins over close friends and premium")
	}

	bots := []*PrivacyRule{
		rule(Predicate_privacyValueDisallowBots, nil, nil),
		rule(Predicate_privacyValueAllowAll, nil, nil),
	}
	if EvaluatePrivacy(bots, PrivacySubject{UserID: 3, Bot: true}) {
		t.Fatal("disallow bots")
	}
	if !EvaluatePrivacy(bots, PrivacySubject{UserID: 4}) {
		t.Fatal("disallow bots still allows a user")
	}
	onlyBots := []*PrivacyRule{rule(Predicate_privacyValueAllowBots, nil, nil)}
	if !EvaluatePrivacy(onlyBots, PrivacySubject{UserID: 3, Bot: true}) {
		t.Fatal("allow bots")
	}
	if EvaluatePrivacy(onlyBots, PrivacySubject{UserID: 4}) {
		t.Fatal("allow bots does not allow a user")
	}
}

func TestEvaluatePrivacyChatParticipants(t *testing.T) {
	allowChat := []*PrivacyRule{
		rule(Predicate_privacyValueAllowChatParticipants, nil, []int64{9}),
		rule(Predicate_privacyValueDisallowAll, nil, nil),
	}
	if !EvaluatePrivacy(allowChat, PrivacySubject{UserID: 3, Member: memberOf(9)}) {
		t.Fatal("member of an allow chat")
	}
	if EvaluatePrivacy(allowChat, PrivacySubject{UserID: 3, Member: memberOf(8)}) {
		t.Fatal("non-member of an allow chat")
	}
	if EvaluatePrivacy(allowChat, PrivacySubject{UserID: 3, Member: func([]int64) (bool, bool) { return false, false }}) {
		t.Fatal("unknown membership must not grant")
	}

	disallowChat := []*PrivacyRule{
		rule(Predicate_privacyValueDisallowChatParticipants, nil, []int64{9}),
		rule(Predicate_privacyValueAllowContacts, nil, nil),
	}
	if EvaluatePrivacy(disallowChat, PrivacySubject{UserID: 3, Contact: true, Member: memberOf(9)}) {
		t.Fatal("member of a disallow chat")
	}
	if !EvaluatePrivacy(disallowChat, PrivacySubject{UserID: 3, Contact: true, Member: memberOf()}) {
		t.Fatal("contact who is not in the chat")
	}
	if !EvaluatePrivacy(disallowChat, PrivacySubject{UserID: 3, Contact: true}) {
		t.Fatal("contact stays allowed when membership cannot be checked")
	}
	if EvaluatePrivacy(disallowChat, PrivacySubject{UserID: 3, Member: memberOf()}) {
		t.Fatal("non-contact")
	}
}

func TestEvaluatePrivacyDisallowContacts(t *testing.T) {
	only := []*PrivacyRule{rule(Predicate_privacyValueDisallowContacts, nil, nil)}
	if EvaluatePrivacy(only, PrivacySubject{UserID: 3, Contact: true}) {
		t.Fatal("disallow contacts denies a contact")
	}
	if !EvaluatePrivacy(only, PrivacySubject{UserID: 3}) {
		t.Fatal("disallow contacts allows a non-contact")
	}
	everybody := []*PrivacyRule{
		rule(Predicate_privacyValueDisallowContacts, nil, nil),
		rule(Predicate_privacyValueAllowAll, nil, nil),
	}
	if EvaluatePrivacy(everybody, PrivacySubject{UserID: 3, Contact: true}) {
		t.Fatal("everybody except contacts")
	}
	if !EvaluatePrivacy(everybody, PrivacySubject{UserID: 4}) {
		t.Fatal("everybody except contacts still allows a stranger")
	}
}

func TestLookupChatMemberWithoutHook(t *testing.T) {
	prev := PrivacyChatMember
	PrivacyChatMember = nil
	t.Cleanup(func() { PrivacyChatMember = prev })
	member, known := LookupChatMember(nil, 3, []int64{9})
	if member || known {
		t.Fatalf("member=%v known=%v", member, known)
	}
}
