package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestImportContactsReturnsRegisteredUser checks contacts.importContacts
// against a user who is already signed up. The address-book spelling and
// the stored spelling of the same test number must both come back in
// imported and users. A bare mainland mobile has no e2e account (those
// numbers are the 99966 test range); matching 13800138000 to 8613800138000
// is covered by PlanContactImport.
func TestImportContactsReturnsRegisteredUser(t *testing.T) {
	a, _, _, bSelf := sharedPair(t)
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(45 * time.Second)
	defer cancel()
	bob, err := cfg.Phone("bob")
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Invoke(ctx, &mtproto.TLContactsImportContacts{
		Constructor: mtproto.CRC32_contacts_importContacts,
		Contacts: []*mtproto.InputContact{
			mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{
				ClientId:  11,
				Phone:     bob.Number,
				FirstName: "Bob",
			}).To_InputContact(),
			mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{
				ClientId:  12,
				Phone:     "13800138000",
				FirstName: "Bare",
			}).To_InputContact(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	imp, ok := res.(*mtproto.TLContactsImportedContacts)
	if !ok {
		t.Fatalf("contacts.importContacts returned %T", res)
	}
	if !importedUser(imp, bSelf.ID) {
		t.Fatalf("imported %#v users %#v", imp.GetImported(), userIDs(imp.GetUsers()))
	}
}

func importedUser(imp *mtproto.TLContactsImportedContacts, id int64) bool {
	var row, user bool
	for _, item := range imp.GetImported() {
		if item != nil && item.GetUserId() == id {
			row = true
		}
	}
	for _, u := range imp.GetUsers() {
		if u != nil && u.GetId() == id {
			user = true
		}
	}
	return row && user
}

func userIDs(users []*mtproto.User) []int64 {
	out := make([]int64, 0, len(users))
	for _, u := range users {
		if u != nil {
			out = append(out, u.GetId())
		}
	}
	return out
}
