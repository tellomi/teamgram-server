package e2e

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// ISSUE-S-ACCEPT-CONTACT-MISSING-001.
//
// contacts.acceptContact returns 400 CONTACT_ADD_MISSING unless the caller's
// peerSettings.add_contact bar is open. The inbox writes that bar, with
// report_spam and block_contact, only for the recipient of the first private
// message from someone who is not already a contact. The sender's own dialog
// is created by that outgoing message, so a reply sees existed=true and does
// not open a bar for the sender. Importing both numbers, or calling
// acceptContact before that message, does not open it either.
// Phones 9996610870–9996610875, code 11111. 9996610820/0821 are the coverage
// pair that hit the missing bar; 0821 is already the notify-exceptions
// account, so this case uses a free range.
// share_contact is a separate bar and is not what these cases open.

func TestAcceptContactMissingBarBothOrders(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610870", "BarA")
	b, bSelf := dialPhone(t, ctx, cfg, "9996610871", "BarB")
	if err := setAddedByPhoneEveryone(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := setAddedByPhoneEveryone(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := setPhoneNobody(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := setPhoneNobody(ctx, b); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = setPhoneEveryone(ctx, a)
		_ = setPhoneEveryone(ctx, b)
	}()
	bID, bHash := resolvePhone(t, ctx, a, "9996610871")
	aID, aHash := resolvePhone(t, ctx, b, "9996610870")
	if aID != aSelf.ID || bID != bSelf.ID {
		t.Fatalf("resolved %d %d want %d %d", aID, bID, aSelf.ID, bSelf.ID)
	}

	// No private message yet. Both orders are the same 400, and neither phone leaks.
	requireAcceptError(t, ctx, a, bID, bHash, 400, "CONTACT_ADD_MISSING")
	requireAcceptError(t, ctx, b, aID, aHash, 400, "CONTACT_ADD_MISSING")
	requireAcceptError(t, ctx, b, aID, aHash, 400, "CONTACT_ADD_MISSING")
	requireAcceptError(t, ctx, a, bID, bHash, 400, "CONTACT_ADD_MISSING")
	requireAcceptError(t, ctx, a, aSelf.ID, aSelf.AccessHash, 400, "CONTACT_ID_INVALID")
	requireAcceptError(t, ctx, a, 0, 0, 400, "CONTACT_ID_INVALID")
	if got := userPhone(t, ctx, b, aID, aHash); got != "" {
		t.Fatalf("phone leaked without the bar: %q", got)
	}
	if flags := peerFlagsNow(t, ctx, a, bID, bHash); flags.GetAddContact() || flags.GetReportSpam() || flags.GetShareContact() {
		t.Fatalf("bar before any message: %#v", flags)
	}

	// Importing the numbers makes them contacts. The first message then must
	// not open add_contact, and acceptContact stays CONTACT_ADD_MISSING.
	if _, _, err := a.ImportPhone(ctx, "9996610871", "BarB"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.ImportPhone(ctx, "9996610870", "BarA"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SendText(ctx, inputPeerUser(bID, bHash), "already contacts", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SendText(ctx, inputPeerUser(aID, aHash), "already contacts", nil); err != nil {
		t.Fatal(err)
	}
	if err := barStaysClosed(ctx, a, bID, bHash); err != nil {
		t.Fatal(err)
	}
	if err := barStaysClosed(ctx, b, aID, aHash); err != nil {
		t.Fatal(err)
	}
	requireAcceptError(t, ctx, a, bID, bHash, 400, "CONTACT_ADD_MISSING")
	requireAcceptError(t, ctx, b, aID, aHash, 400, "CONTACT_ADD_MISSING")
}

func TestAcceptContactStrangerBarBothOrders(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	// Fresh pair: 0872 sends the only message, 0873 is the recipient who can accept.
	acceptOneWay(t, ctx, cfg, "9996610872", "SndA", "9996610873", "RcvB")
	// Fresh pair, opposite initiator. A reply in the first pair must not be required.
	acceptOneWay(t, ctx, cfg, "9996610875", "SndD", "9996610874", "RcvC")
}

// acceptOneWay is one first private message. Only the recipient gets
// report_spam, add_contact, and block_contact. The sender's acceptContact
// stays 400 CONTACT_ADD_MISSING.
func acceptOneWay(t *testing.T, ctx context.Context, cfg Config, senderPhone, senderName, recipientPhone, recipientName string) {
	t.Helper()
	sender, senderSelf := dialPhone(t, ctx, cfg, senderPhone, senderName)
	recipient, recipientSelf := dialPhone(t, ctx, cfg, recipientPhone, recipientName)
	if err := setAddedByPhoneEveryone(ctx, sender); err != nil {
		t.Fatal(err)
	}
	if err := setAddedByPhoneEveryone(ctx, recipient); err != nil {
		t.Fatal(err)
	}
	if err := setPhoneNobody(ctx, sender); err != nil {
		t.Fatal(err)
	}
	if err := setPhoneNobody(ctx, recipient); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = setPhoneEveryone(ctx, sender)
		_ = setPhoneEveryone(ctx, recipient)
	}()
	recipientID, recipientHash := resolvePhone(t, ctx, sender, recipientPhone)
	senderID, senderHash := resolvePhone(t, ctx, recipient, senderPhone)
	if senderID != senderSelf.ID || recipientID != recipientSelf.ID {
		t.Fatalf("resolved sender %d recipient %d", senderID, recipientID)
	}
	if _, err := sender.SendText(ctx, inputPeerUser(recipientID, recipientHash), "hello once", nil); err != nil {
		t.Fatal(err)
	}
	bar := waitStrangerBar(t, ctx, recipient, senderID, senderHash)
	if bar.GetShareContact() {
		t.Fatalf("share_contact on the stranger bar %#v", bar)
	}
	if err := barStaysClosed(ctx, sender, recipientID, recipientHash); err != nil {
		t.Fatal(err)
	}
	requireAcceptError(t, ctx, sender, recipientID, recipientHash, 400, "CONTACT_ADD_MISSING")
	if got := userPhone(t, ctx, sender, recipientID, recipientHash); got != "" {
		t.Fatalf("recipient phone visible before accept: %q", got)
	}
	if got := userPhone(t, ctx, recipient, senderID, senderHash); got != "" {
		t.Fatalf("sender phone visible before accept: %q", got)
	}

	out := acceptContact(t, ctx, recipient, senderID, senderHash)
	if !acceptedUser(out, senderID) || !clearedBar(out, senderID) {
		t.Fatalf("accept %#v", out)
	}
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return userPhone(t, ctx, sender, recipientID, recipientHash) != "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := userPhone(t, ctx, recipient, senderID, senderHash); got != "" {
		t.Fatalf("accept shared the sender phone: %q", got)
	}
	flags := peerFlagsNow(t, ctx, recipient, senderID, senderHash)
	if flags.GetAddContact() || flags.GetReportSpam() || flags.GetBlockContact() || flags.GetShareContact() {
		t.Fatalf("bar left open %#v", flags)
	}
	for i := 0; i < 2; i++ {
		requireAcceptError(t, ctx, recipient, senderID, senderHash, 400, "CONTACT_ADD_MISSING")
	}
	if got := userPhone(t, ctx, sender, recipientID, recipientHash); got == "" {
		t.Fatal("repeat cleared the shared phone")
	}
	if user := userByID(usersOf(mustGetUsers(t, ctx, recipient, senderID, senderHash)), senderID); user == nil || !user.GetContact() {
		t.Fatal("repeat dropped the contact")
	}
	requireAcceptError(t, ctx, sender, recipientID, recipientHash, 400, "CONTACT_ADD_MISSING")
	if err := barStaysClosed(ctx, sender, recipientID, recipientHash); err != nil {
		t.Fatal(err)
	}
}

func acceptContact(t *testing.T, ctx context.Context, c *Client, id, hash int64) mtproto.TLObject {
	t.Helper()
	out, err := c.Invoke(ctx, &mtproto.TLContactsAcceptContact{
		Constructor: mtproto.CRC32_contacts_acceptContact,
		Id:          inputUser(id, hash),
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func requireAcceptError(t *testing.T, ctx context.Context, c *Client, id, hash int64, code int, message string) {
	t.Helper()
	_, err := c.Invoke(ctx, &mtproto.TLContactsAcceptContact{
		Constructor: mtproto.CRC32_contacts_acceptContact,
		Id:          inputUser(id, hash),
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Code != code || rpc.Message != message {
		t.Fatalf("acceptContact(%d) = %v want %d %s", id, err, code, message)
	}
}

func resolvePhone(t *testing.T, ctx context.Context, c *Client, phone string) (int64, int64) {
	t.Helper()
	res, err := c.Invoke(ctx, &mtproto.TLContactsResolvePhone{
		Constructor: mtproto.CRC32_contacts_resolvePhone,
		Phone:       phone,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := res.(*mtproto.TLContactsResolvedPeer)
	if !ok {
		t.Fatalf("contacts.resolvePhone returned %T", res)
	}
	want := digitsOnly(phone)
	for _, user := range resolved.GetUsers() {
		if user == nil || user.GetId() == 0 {
			continue
		}
		got := ""
		if user.GetPhone() != nil {
			got = digitsOnly(user.GetPhone().GetValue())
		}
		if got != "" && got != want {
			continue
		}
		hash := int64(0)
		if user.GetAccessHash() != nil {
			hash = user.GetAccessHash().GetValue()
		}
		return user.GetId(), hash
	}
	t.Fatalf("contacts.resolvePhone did not return %s", phone)
	return 0, 0
}

func setAddedByPhoneEveryone(ctx context.Context, c *Client) error {
	_, err := c.Invoke(ctx, &mtproto.TLAccountSetPrivacy{
		Constructor: mtproto.CRC32_account_setPrivacy,
		Key:         mtproto.MakeTLInputPrivacyKeyAddedByPhone(&mtproto.InputPrivacyKey{}).To_InputPrivacyKey(),
		Rules: []*mtproto.InputPrivacyRule{
			mtproto.MakeTLInputPrivacyValueAllowAll(&mtproto.InputPrivacyRule{}).To_InputPrivacyRule(),
		},
	})
	return err
}

func waitStrangerBar(t *testing.T, ctx context.Context, c *Client, id, hash int64) *mtproto.PeerSettings {
	t.Helper()
	var got *mtproto.PeerSettings
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		settings, err := loadPeerSettings(ctx, c, id, hash)
		if err != nil {
			return false, err
		}
		got = settings
		if settings == nil {
			return false, nil
		}
		return settings.GetReportSpam() && settings.GetAddContact() && settings.GetBlockContact(), nil
	}); err != nil {
		t.Fatalf("stranger bar: %v %#v", err, got)
	}
	return got
}

func barStaysClosed(ctx context.Context, c *Client, id, hash int64) error {
	short, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := waitUntil(short, 200*time.Millisecond, func() (bool, error) {
		settings, err := loadPeerSettings(short, c, id, hash)
		if err != nil {
			return false, err
		}
		if settings.GetAddContact() || settings.GetReportSpam() || settings.GetBlockContact() {
			return false, errors.New("stranger bar opened for an existing contact")
		}
		return false, nil
	})
	if errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func peerFlagsNow(t *testing.T, ctx context.Context, c *Client, id, hash int64) *mtproto.PeerSettings {
	t.Helper()
	settings, err := loadPeerSettings(ctx, c, id, hash)
	if err != nil {
		t.Fatal(err)
	}
	if settings == nil {
		return &mtproto.PeerSettings{}
	}
	return settings
}

func mustGetUsers(t *testing.T, ctx context.Context, c *Client, id, hash int64) mtproto.TLObject {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id:          []*mtproto.InputUser{inputUser(id, hash)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return obj
}

func acceptedUser(obj mtproto.TLObject, id int64) bool {
	user := userByID(usersOf(obj), id)
	return user != nil && user.GetContact()
}

func clearedBar(obj mtproto.TLObject, id int64) bool {
	for _, update := range updatesOf(obj) {
		if update.GetPeer_PEER().GetUserId() != id {
			continue
		}
		settings := update.GetSettings()
		if settings == nil {
			continue
		}
		return !settings.GetAddContact() && !settings.GetReportSpam() && !settings.GetBlockContact() && !settings.GetShareContact()
	}
	return false
}
