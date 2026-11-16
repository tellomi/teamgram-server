package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// Phones 9996610466–468 are this task's range.

func TestEditCloseFriendsReplacesTheList(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610466", "Closer")
	_, bob := dialPhone(t, ctx, cfg, "9996610467", "Bob")
	_, cara := dialPhone(t, ctx, cfg, "9996610468", "Cara")
	if _, err := owner.Invoke(ctx, &mtproto.TLContactsImportContacts{
		Constructor: mtproto.CRC32_contacts_importContacts,
		Contacts: []*mtproto.InputContact{
			mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{
				ClientId:  1,
				Phone:     "9996610467",
				FirstName: "Bob",
			}).To_InputContact(),
			mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{
				ClientId:  2,
				Phone:     "9996610468",
				FirstName: "Cara",
			}).To_InputContact(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	editClose(t, ctx, owner, bob.ID, cara.ID)
	if !closeFriend(t, ctx, owner, bob) || !closeFriend(t, ctx, owner, cara) {
		t.Fatal("both contacts should be close friends")
	}
	editClose(t, ctx, owner, bob.ID)
	if !closeFriend(t, ctx, owner, bob) || closeFriend(t, ctx, owner, cara) {
		t.Fatal("replacing the list did not remove Cara")
	}
	editClose(t, ctx, owner)
	if closeFriend(t, ctx, owner, bob) || closeFriend(t, ctx, owner, cara) {
		t.Fatal("an empty list did not clear close friends")
	}
}

func editClose(t *testing.T, ctx context.Context, c *Client, ids ...int64) {
	t.Helper()
	if _, err := c.Invoke(ctx, &mtproto.TLContactsEditCloseFriends{
		Constructor: mtproto.CRC32_contacts_editCloseFriends,
		Id:          ids,
	}); err != nil {
		t.Fatal(err)
	}
}

func closeFriend(t *testing.T, ctx context.Context, c *Client, who Self) bool {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id:          []*mtproto.InputUser{inputUser(who.ID, who.AccessHash)},
	})
	if err != nil {
		t.Fatal(err)
	}
	user := userByID(usersOf(obj), who.ID)
	if user == nil {
		t.Fatal("missing user")
	}
	return user.GetCloseFriend()
}
