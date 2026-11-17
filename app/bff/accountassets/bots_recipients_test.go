package accountassets

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestBusinessBarUsesRealRecipientCategories(t *testing.T) {
	cases := []struct {
		name    string
		flags   *mtproto.InputBusinessBotRecipients
		users   []int64
		exclude []int64
		facts   ViewerFacts
		viewer  int64
		show    bool
	}{
		{name: "contacts shows a contact", flags: &mtproto.InputBusinessBotRecipients{Contacts: true}, facts: ViewerFacts{Contact: true, ContactOK: true}, viewer: 9, show: true},
		{name: "contacts hides a non-contact", flags: &mtproto.InputBusinessBotRecipients{Contacts: true}, facts: ViewerFacts{Contact: false, ContactOK: true}, viewer: 9, show: false},
		{name: "non-contacts shows a non-contact", flags: &mtproto.InputBusinessBotRecipients{NonContacts: true}, facts: ViewerFacts{Contact: false, ContactOK: true}, viewer: 9, show: true},
		{name: "non-contacts hides a contact", flags: &mtproto.InputBusinessBotRecipients{NonContacts: true}, facts: ViewerFacts{Contact: true, ContactOK: true}, viewer: 9, show: false},
		{name: "existing chats shows an open dialog", flags: &mtproto.InputBusinessBotRecipients{ExistingChats: true}, facts: ViewerFacts{ExistingChat: true, ChatOK: true}, viewer: 9, show: true},
		{name: "existing chats hides a new dialog", flags: &mtproto.InputBusinessBotRecipients{ExistingChats: true}, facts: ViewerFacts{ExistingChat: false, ChatOK: true}, viewer: 9, show: false},
		{name: "new chats shows a new dialog", flags: &mtproto.InputBusinessBotRecipients{NewChats: true}, facts: ViewerFacts{ExistingChat: false, ChatOK: true}, viewer: 9, show: true},
		{name: "new chats hides an open dialog", flags: &mtproto.InputBusinessBotRecipients{NewChats: true}, facts: ViewerFacts{ExistingChat: true, ChatOK: true}, viewer: 9, show: false},
		{name: "unknown contact is not treated as everyone", flags: &mtproto.InputBusinessBotRecipients{Contacts: true}, facts: ViewerFacts{}, viewer: 9, show: false},
		{name: "unknown dialog is not treated as everyone", flags: &mtproto.InputBusinessBotRecipients{ExistingChats: true, NewChats: true}, facts: ViewerFacts{}, viewer: 9, show: false},
		{name: "users list shows someone outside the categories", flags: &mtproto.InputBusinessBotRecipients{Contacts: true}, users: []int64{9}, facts: ViewerFacts{Contact: false, ContactOK: true}, viewer: 9, show: true},
		{name: "users list does not show a different person", flags: &mtproto.InputBusinessBotRecipients{}, users: []int64{8}, facts: ViewerFacts{Contact: true, ContactOK: true, ExistingChat: true, ChatOK: true}, viewer: 9, show: false},
		{name: "exclude_users hides a matching contact", flags: &mtproto.InputBusinessBotRecipients{Contacts: true}, exclude: []int64{9}, facts: ViewerFacts{Contact: true, ContactOK: true}, viewer: 9, show: false},
		{name: "exclude_users hides an explicitly listed user", flags: &mtproto.InputBusinessBotRecipients{}, users: []int64{9}, exclude: []int64{9}, facts: ViewerFacts{ContactOK: true, ChatOK: true}, viewer: 9, show: false},
		{name: "exclude_selected removes the listed contact", flags: &mtproto.InputBusinessBotRecipients{Contacts: true, ExcludeSelected: true}, users: []int64{9}, facts: ViewerFacts{Contact: true, ContactOK: true}, viewer: 9, show: false},
		{name: "exclude_selected keeps an unlisted contact", flags: &mtproto.InputBusinessBotRecipients{Contacts: true, ExcludeSelected: true}, users: []int64{8}, facts: ViewerFacts{Contact: true, ContactOK: true}, viewer: 9, show: true},
		{name: "exclude_selected does not add a listed non-contact", flags: &mtproto.InputBusinessBotRecipients{Contacts: true, ExcludeSelected: true}, users: []int64{9}, facts: ViewerFacts{Contact: false, ContactOK: true}, viewer: 9, show: false},
		{name: "contact or existing chat", flags: &mtproto.InputBusinessBotRecipients{Contacts: true, ExistingChats: true}, facts: ViewerFacts{Contact: false, ContactOK: true, ExistingChat: true, ChatOK: true}, viewer: 9, show: true},
		{name: "neither contact nor existing chat", flags: &mtproto.InputBusinessBotRecipients{Contacts: true, ExistingChats: true}, facts: ViewerFacts{Contact: false, ContactOK: true, ExistingChat: false, ChatOK: true}, viewer: 9, show: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Reset()
			ViewerFactsOf = func(int64, int64) ViewerFacts { return tc.facts }
			t.Cleanup(func() { ViewerFactsOf = nil })
			in := *tc.flags
			for _, id := range tc.users {
				in.Users = append(in.Users, mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id, AccessHash: 1}).To_InputUser())
			}
			for _, id := range tc.exclude {
				in.ExcludeUsers = append(in.ExcludeUsers, mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id, AccessHash: 1}).To_InputUser())
			}
			connectBot(t, &in)
			settings := mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{}).To_PeerSettings()
			bar := ApplyBusinessBar(1, tc.viewer, settings)
			if tc.show {
				if bar == nil || settings.GetBusinessBotId().GetValue() != 7 || settings.GetBusinessBotManageUrl().GetValue() == "" {
					t.Fatalf("bar hidden: %#v %#v", bar, settings)
				}
				return
			}
			if bar != nil || settings.GetBusinessBotId() != nil || settings.GetBusinessBotManageUrl() != nil {
				t.Fatalf("bar leaked: %#v %#v", bar, settings)
			}
		})
	}
}

func connectBot(t *testing.T, recipients *mtproto.InputBusinessBotRecipients) {
	t.Helper()
	bot := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 7, AccessHash: 1}).To_InputUser()
	if _, err := UpdateConnectedBot(1, &mtproto.TLAccountUpdateConnectedBot{
		Bot:                                   bot,
		Recipients_INPUTBUSINESSBOTRECIPIENTS: mtproto.MakeTLInputBusinessBotRecipients(recipients).To_InputBusinessBotRecipients(),
		Rights:                                mtproto.MakeTLBusinessBotRights(&mtproto.BusinessBotRights{Reply: true}).To_BusinessBotRights(),
	}); err != nil {
		t.Fatal(err)
	}
}
