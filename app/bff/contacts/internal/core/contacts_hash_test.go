package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestContactsHashNotModifiedAndDeleteByPhones(t *testing.T) {
	mem := &dao.MemSaved{
		Unregistered: map[int64][]dao.SavedPhone{
			4: {{Phone: "+86 139 0000 0000", FirstName: "Bea"}},
		},
	}
	c := relCore(4, mem)
	rows := []*mtproto.ContactData{
		{ContactUserId: 9, Phone: wrapperspb.String("8613900139000")},
		{ContactUserId: 8, Phone: wrapperspb.String("8613800138000")},
	}
	prevFetch, prevDrop := fetchContactRows, dropContactIDs
	t.Cleanup(func() {
		fetchContactRows = prevFetch
		dropContactIDs = prevDrop
	})
	fetchContactRows = func(*ContactsCore) ([]*mtproto.ContactData, error) { return rows, nil }
	var dropped []int64
	dropContactIDs = func(_ *ContactsCore, ids []int64) error {
		dropped = append([]int64(nil), ids...)
		return nil
	}

	if _, err := c.ContactsDeleteByPhones(&mtproto.TLContactsDeleteByPhones{
		Phones: []string{"+86 138-0013-8000"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(dropped) != 1 || dropped[0] != 8 {
		t.Fatalf("deleted %#v", dropped)
	}
	if n, err := mem.UnregisteredCount(context.Background(), 4); err != nil || n != 1 {
		t.Fatalf("unrelated saved row removed: %d %v", n, err)
	}
	if _, err := c.ContactsDeleteByPhones(&mtproto.TLContactsDeleteByPhones{
		Phones: []string{"8613900000000"},
	}); err != nil {
		t.Fatal(err)
	}
	if n, _ := mem.UnregisteredCount(context.Background(), 4); n != 0 {
		t.Fatalf("saved phone remained: %d", n)
	}

	contacts := []*mtproto.Contact{
		{UserId: 9},
		{UserId: 8},
	}
	hash := contactsHash(c.savedCount(), contacts)
	if hash == 0 {
		t.Fatal("hash")
	}
	got, err := c.ContactsGetContacts(&mtproto.TLContactsGetContacts{Hash: hash})
	if err != nil || got.GetPredicateName() != mtproto.Predicate_contacts_contactsNotModified {
		t.Fatalf("not modified: %v %#v", err, got)
	}
	if again := contactsHash(c.savedCount(), []*mtproto.Contact{{UserId: 8}, {UserId: 9}}); again != hash {
		t.Fatalf("order %d vs %d", again, hash)
	}
}
