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
	saved, err := c.savedCount()
	if err != nil {
		t.Fatal(err)
	}
	hash := contactsHash(saved, contacts)
	// Android calcHash, saved_count 0, user ids 8 then 9.
	if hash != 292057776145 {
		t.Fatalf("android hash %d", hash)
	}
	got, err := c.ContactsGetContacts(&mtproto.TLContactsGetContacts{Hash: hash})
	if err != nil || got.GetPredicateName() != mtproto.Predicate_contacts_contactsNotModified {
		t.Fatalf("not modified: %v %#v", err, got)
	}
	if again := contactsHash(saved, []*mtproto.Contact{{UserId: 8}, {UserId: 9}}); again != hash {
		t.Fatalf("order %d vs %d", again, hash)
	}
}

func TestContactsHashAndroidVectors(t *testing.T) {
	cases := []struct {
		saved int32
		ids   []int64
		want  int64
	}{
		{0, nil, 0},
		{1, nil, 1},
		{0, []int64{1}, 1},
		{3, []int64{9, 8}, 1695745832960212},
		{1, []int64{136907799}, 36644129816},
	}
	for _, tc := range cases {
		contacts := make([]*mtproto.Contact, 0, len(tc.ids))
		for _, id := range tc.ids {
			contacts = append(contacts, &mtproto.Contact{UserId: id})
		}
		if got := contactsHash(tc.saved, contacts); got != tc.want {
			t.Fatalf("saved %d ids %v: got %d want %d", tc.saved, tc.ids, got, tc.want)
		}
	}
}

func TestGetContactsOmitsUnresolvedUserAndThenNotModified(t *testing.T) {
	c := relCore(4, nil)
	rows := []*mtproto.ContactData{
		{ContactUserId: 9, MutualContact: true},
		{ContactUserId: 9},
		{ContactUserId: 8},
	}
	prevFetch, prevLoad := fetchContactRows, loadContactUsers
	t.Cleanup(func() {
		fetchContactRows = prevFetch
		loadContactUsers = prevLoad
	})
	fetchContactRows = func(*ContactsCore) ([]*mtproto.ContactData, error) { return rows, nil }
	loadContactUsers = func(_ *ContactsCore, ids []int64) ([]*mtproto.User, error) {
		var out []*mtproto.User
		for _, id := range ids {
			if id == 9 {
				continue
			}
			out = append(out, mtproto.MakeTLUser(&mtproto.User{Id: id, Contact: true}).To_User())
		}
		return out, nil
	}

	got, err := c.ContactsGetContacts(&mtproto.TLContactsGetContacts{})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_contacts_contacts {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
	if len(got.GetContacts()) != 1 || got.GetContacts()[0].GetUserId() != 8 {
		t.Fatalf("contacts %#v", got.GetContacts())
	}
	if len(got.GetUsers()) != 1 || got.GetUsers()[0].GetId() != 8 {
		t.Fatalf("users %#v", got.GetUsers())
	}
	hash := contactsHash(got.GetSavedCount(), got.GetContacts())
	again, err := c.ContactsGetContacts(&mtproto.TLContactsGetContacts{Hash: hash})
	if err != nil || again.GetPredicateName() != mtproto.Predicate_contacts_contactsNotModified {
		t.Fatalf("second fetch %v %#v", err, again)
	}
}

func TestGetContactsDoesNotWipeBookWhenUsersMissing(t *testing.T) {
	c := relCore(4, nil)
	prevFetch, prevLoad := fetchContactRows, loadContactUsers
	t.Cleanup(func() {
		fetchContactRows = prevFetch
		loadContactUsers = prevLoad
	})
	fetchContactRows = func(*ContactsCore) ([]*mtproto.ContactData, error) {
		return []*mtproto.ContactData{{ContactUserId: 8}}, nil
	}
	loadContactUsers = func(*ContactsCore, []int64) ([]*mtproto.User, error) {
		return nil, nil
	}
	_, err := c.ContactsGetContacts(&mtproto.TLContactsGetContacts{})
	if err == nil {
		t.Fatal("empty users must not be a successful empty book")
	}
}

func TestGetContactsListErrorKeepsBookWhenHashSet(t *testing.T) {
	c := relCore(4, nil)
	prev := fetchContactRows
	t.Cleanup(func() { fetchContactRows = prev })
	fetchContactRows = func(*ContactsCore) ([]*mtproto.ContactData, error) {
		return nil, context.DeadlineExceeded
	}
	if _, err := c.ContactsGetContacts(&mtproto.TLContactsGetContacts{Hash: 99}); err == nil {
		t.Fatal("non-zero hash must not replace the book with an empty success")
	}
	got, err := c.ContactsGetContacts(&mtproto.TLContactsGetContacts{})
	if err != nil || got.GetPredicateName() != mtproto.Predicate_contacts_contacts || len(got.GetContacts()) != 0 {
		t.Fatalf("hash 0 startup: %v %#v", err, got)
	}
}

func TestGetContactIDsUsesContactsHash(t *testing.T) {
	c := relCore(4, nil)
	prev := fetchContactRows
	t.Cleanup(func() { fetchContactRows = prev })
	fetchContactRows = func(*ContactsCore) ([]*mtproto.ContactData, error) {
		return []*mtproto.ContactData{
			{ContactUserId: 9},
			{ContactUserId: 8},
			{ContactUserId: 8},
		}, nil
	}
	got, err := c.ContactsGetContactIDs(&mtproto.TLContactsGetContactIDs{})
	if err != nil || len(got.GetDatas()) != 2 || got.GetDatas()[0] != 8 || got.GetDatas()[1] != 9 {
		t.Fatalf("ids %#v %v", got.GetDatas(), err)
	}
	hash := contactsHash(0, []*mtproto.Contact{{UserId: 8}, {UserId: 9}})
	if hash != 292057776145 {
		t.Fatalf("hash %d", hash)
	}
	same, err := c.ContactsGetContactIDs(&mtproto.TLContactsGetContactIDs{Hash: hash})
	if err != nil || len(same.GetDatas()) != 0 {
		t.Fatalf("not modified %#v %v", same.GetDatas(), err)
	}
	again, err := c.ContactsGetContactIDs(&mtproto.TLContactsGetContactIDs{Hash: hash + 1})
	if err != nil || len(again.GetDatas()) != 2 {
		t.Fatalf("mismatch %#v %v", again.GetDatas(), err)
	}
}

func TestGetContactsSavedCountErrorIsNotZero(t *testing.T) {
	c := relCore(4, errSaved{})
	prev := fetchContactRows
	t.Cleanup(func() { fetchContactRows = prev })
	fetchContactRows = func(*ContactsCore) ([]*mtproto.ContactData, error) {
		return []*mtproto.ContactData{{ContactUserId: 8}}, nil
	}
	_, err := c.ContactsGetContacts(&mtproto.TLContactsGetContacts{Hash: contactsHash(0, []*mtproto.Contact{{UserId: 8}})})
	if err == nil {
		t.Fatal("saved_count error must not compare as 0")
	}
}

type errSaved struct{}

func (errSaved) List(context.Context, int64) ([]dao.SavedPhone, error) { return nil, nil }
func (errSaved) ResetUnregistered(context.Context, int64) error        { return nil }
func (errSaved) DeletePhones(context.Context, int64, []string) error   { return nil }
func (errSaved) UnregisteredCount(context.Context, int64) (int32, error) {
	return 0, context.DeadlineExceeded
}
