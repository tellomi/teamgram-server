package mtproto

import "testing"

func TestPrivacyChatParticipantRulesDoNotGrantBlindly(t *testing.T) {
	allowChat := []*PrivacyRule{
		MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
		MakeTLPrivacyValueAllowChatParticipants(&PrivacyRule{Chats: []int64{9}}).To_PrivacyRule(),
	}
	if privacyIsAllow(allowChat, 3, false) {
		t.Fatal("allow chat participants must not allow a user without a membership check")
	}
	disallowChat := []*PrivacyRule{
		MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule(),
		MakeTLPrivacyValueDisallowChatParticipants(&PrivacyRule{Chats: []int64{9}}).To_PrivacyRule(),
	}
	if privacyIsAllow(disallowChat, 3, false) {
		t.Fatal("disallow chat participants must not allow a non-contact")
	}
	if !privacyIsAllow(disallowChat, 3, true) {
		t.Fatal("a contact stays allowed when chat membership cannot be checked")
	}
	both := []*PrivacyRule{
		MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule(),
		MakeTLPrivacyValueDisallowChatParticipants(&PrivacyRule{Chats: []int64{9}}).To_PrivacyRule(),
		MakeTLPrivacyValueDisallowUsers(&PrivacyRule{Users: []int64{3}}).To_PrivacyRule(),
	}
	if privacyIsAllow(both, 3, true) {
		t.Fatal("disallow user must still deny when a chat rule is also present")
	}
}
