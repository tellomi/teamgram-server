package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Phones 9996610463 and 9996610464 are this task's range.

func TestContactBirthdays(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610463", "Bday")
	friend, friendSelf := dialPhone(t, ctx, cfg, "9996610464", "BdayPal")
	if _, err := friend.Invoke(ctx, &mtproto.TLAccountUpdateBirthday{
		Constructor: mtproto.CRC32_account_updateBirthday,
		Birthday: mtproto.MakeTLBirthday(&mtproto.Birthday{
			Day:   3,
			Month: 4,
			Year:  wrapperspb.Int32(1991),
		}).To_Birthday(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Invoke(ctx, &mtproto.TLContactsImportContacts{
		Constructor: mtproto.CRC32_contacts_importContacts,
		Contacts: []*mtproto.InputContact{
			mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{
				ClientId:  1,
				Phone:     "9996610464",
				FirstName: "BdayPal",
			}).To_InputContact(),
		},
	}); err != nil {
		t.Fatal(err)
	}
	obj, err := owner.Invoke(ctx, &mtproto.TLContactsGetBirthdays{
		Constructor: mtproto.CRC32_contacts_getBirthdays,
	})
	if err != nil {
		t.Fatal(err)
	}
	var rows []*mtproto.ContactBirthday
	switch v := obj.(type) {
	case *mtproto.TLContactsContactBirthdays:
		rows = v.GetContacts()
	case *mtproto.Contacts_ContactBirthdays:
		rows = v.GetContacts()
	default:
		t.Fatalf("contacts.getBirthdays %T", obj)
	}
	for _, row := range rows {
		if row.GetContactId() == friendSelf.ID && row.GetBirthday().GetDay() == 3 && row.GetBirthday().GetMonth() == 4 {
			return
		}
	}
	t.Fatalf("missing birthday for %d in %+v", friendSelf.ID, rows)
}

func TestBoostURLNotEmpty(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, self := dialPhone(t, ctx, cfg, "9996610463", "Boost")
	obj, err := c.Invoke(ctx, &mtproto.TLPremiumGetBoostsStatus{
		Constructor: mtproto.CRC32_premium_getBoostsStatus,
		Peer:        inputPeerSelf(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var url string
	switch v := obj.(type) {
	case *mtproto.TLPremiumBoostsStatus:
		url = v.GetBoostUrl()
	case *mtproto.Premium_BoostsStatus:
		url = v.GetBoostUrl()
	default:
		t.Fatalf("premium.getBoostsStatus %T", obj)
	}
	want := "https://t.me/boost?c="
	if len(url) < len(want) || url[:len(want)] != want {
		t.Fatalf("boost_url %q for %d", url, self.ID)
	}
}
