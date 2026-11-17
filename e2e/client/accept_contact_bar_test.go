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
// peerSettings.add_contact bar is open. That bar is the first private message
// from someone who is not already a contact, and it arrives with report_spam
// and block_contact. Importing both numbers, or calling acceptContact before
// that message, does not open it. Phones 9996610870–9996610875, code 11111.
// 9996610820/0821 are the coverage pair that hit the missing bar; 0821 is
// already the notify-exceptions account, so this case uses a free range.

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
	ctx, cancel := contextTimeout(4 * time.Minute)
	defer cancel()
	// First pair: A accepts B, then B accepts A.
	acceptStrangerPair(t, ctx, cfg, "9996610872", "OrdA", "9996610873", "OrdB", true)
	// Second pair: the other order. The first accept must not be required.
	acceptStrangerPair(t, ctx, cfg, "9996610874", "RevA", "9996610875", "RevB", false)
}

func acceptStrangerPair(t *testing.T, ctx context.Context, cfg Config, phoneA, nameA, phoneB, nameB string, aFirst bool) {
	t.Helper()
	a, aSelf := dialPhone(t, ctx, cfg, phoneA, nameA)
	b, bSelf := dialPhone(t, ctx, cfg, phoneB, nameB)
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
	// resolvePhone does not import the contact, so the first message is from
	// a non-contact and opens the bar on each side.
	bID, bHash := resolvePhone(t, ctx, a, phoneB)
	aID, aHash := resolvePhone(t, ctx, b, phoneA)
	if aID != aSelf.ID || bID != bSelf.ID {
		t.Fatalf("resolved %d %d", aID, bID)
	}
	if _, err := a.SendText(ctx, inputPeerUser(bID, bHash), "hello from a", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SendText(ctx, inputPeerUser(aID, aHash), "hello from b", nil); err != nil {
		t.Fatal(err)
	}
	barA := waitStrangerBar(t, ctx, a, bID, bHash)
	barB := waitStrangerBar(t, ctx, b, aID, aHash)
	if barA.GetShareContact() || barB.GetShareContact() {
		t.Fatalf("share_contact on stranger bars %#v %#v", barA, barB)
	}

	first, second := a, b
	firstPeer, firstHash := bID, bHash
	secondPeer, secondHash := aID, aHash
	firstPhoneSeenBy := b
	firstPhoneID, firstPhoneHash := aID, aHash
	secondPhoneSeenBy := a
	secondPhoneID, secondPhoneHash := bID, bHash
	if !aFirst {
		first, second = b, a
		firstPeer, firstHash = aID, aHash
		secondPeer, secondHash = bID, bHash
		firstPhoneSeenBy = a
		firstPhoneID, firstPhoneHash = bID, bHash
		secondPhoneSeenBy = b
		secondPhoneID, secondPhoneHash = aID, aHash
	}
	out := acceptContact(t, ctx, first, firstPeer, firstHash)
	if !acceptedUser(out, firstPeer) || !clearedBar(out, firstPeer) {
		t.Fatalf("first accept %#v", out)
	}
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return userPhone(t, ctx, firstPhoneSeenBy, firstPhoneID, firstPhoneHash) != "", nil
	}); err != nil {
		t.Fatal(err)
	}
	requireAcceptError(t, ctx, first, firstPeer, firstHash, 400, "CONTACT_ADD_MISSING")
	if got := userPhone(t, ctx, firstPhoneSeenBy, firstPhoneID, firstPhoneHash); got == "" {
		t.Fatal("repeat cleared the shared phone")
	}

	out = acceptContact(t, ctx, second, secondPeer, secondHash)
	if !acceptedUser(out, secondPeer) || !clearedBar(out, secondPeer) {
		t.Fatalf("second accept %#v", out)
	}
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return userPhone(t, ctx, secondPhoneSeenBy, secondPhoneID, secondPhoneHash) != "", nil
	}); err != nil {
		t.Fatal(err)
	}
	requireAcceptError(t, ctx, second, secondPeer, secondHash, 400, "CONTACT_ADD_MISSING")
	if flags := peerFlagsNow(t, ctx, a, bID, bHash); flags.GetAddContact() || flags.GetReportSpam() || flags.GetBlockContact() {
		t.Fatalf("bar left open %#v", flags)
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
