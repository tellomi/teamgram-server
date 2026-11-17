package e2e

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestStrangerBarThenHide is the first private message from someone who is
// not a contact. The recipient's messages.getPeerSettings carries
// report_spam, add_contact, and block_contact. hidePeerSettingsBar clears them.
func TestStrangerBarThenHide(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "bar_a", "BarA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "bar_b", "BarB")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	phone, err := cfg.Phone("bar_b")
	if err != nil {
		t.Fatal(err)
	}
	bID, bHash, err := a.ImportPhone(ctx, phone.Number, "BarB")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.SendText(ctx, inputPeerUser(bID, bHash), "hello stranger", nil); err != nil {
		t.Fatal(err)
	}
	aHash := waitUserHash(t, ctx, b, aSelf.ID)
	flags := waitPeerFlags(t, ctx, b, aSelf.ID, aHash, true)
	if !flags.GetReportSpam() || !flags.GetAddContact() || !flags.GetBlockContact() {
		t.Fatalf("bar %#v", flags)
	}
	if _, err = b.Invoke(ctx, &mtproto.TLMessagesHidePeerSettingsBar{
		Constructor: mtproto.CRC32_messages_hidePeerSettingsBar,
		Peer:        inputPeerUser(aSelf.ID, aHash),
	}); err != nil {
		t.Fatal(err)
	}
	cleared := waitPeerFlags(t, ctx, b, aSelf.ID, aHash, false)
	if cleared.GetReportSpam() || cleared.GetAddContact() || cleared.GetBlockContact() {
		t.Fatalf("bar remained %#v", cleared)
	}
}

// TestAddContactClearsBarAndSharesPhone is contacts.addContact with
// add_phone_privacy_exception. The stranger bar goes away, and the user who
// was added can see the adder's phone after the phone privacy was nobody.
func TestAddContactClearsBarAndSharesPhone(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "bar_a", "BarA")
	c, _ := loginNamed(t, cfg, ProfileAndroid, "bar_c", "BarC")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := setPhoneNobody(ctx, c); err != nil {
		t.Fatal(err)
	}
	phone, err := cfg.Phone("bar_c")
	if err != nil {
		t.Fatal(err)
	}
	cID, cHash, err := a.ImportPhone(ctx, phone.Number, "BarC")
	if err != nil {
		t.Fatal(err)
	}
	if got := userPhone(t, ctx, a, cID, cHash); got != "" {
		t.Fatalf("phone visible before the exception: %q", got)
	}
	if _, err = a.SendText(ctx, inputPeerUser(cID, cHash), "first", nil); err != nil {
		t.Fatal(err)
	}
	aHash := waitUserHash(t, ctx, c, aSelf.ID)
	if flags := waitPeerFlags(t, ctx, c, aSelf.ID, aHash, true); !flags.GetAddContact() || !flags.GetReportSpam() || !flags.GetBlockContact() {
		t.Fatalf("bar %#v", flags)
	}
	if _, err = c.Invoke(ctx, &mtproto.TLContactsAddContact{
		Constructor:              mtproto.CRC32_contacts_addContact_d9ba2e54,
		AddPhonePrivacyException: true,
		Id:                       inputUser(aSelf.ID, aHash),
		FirstName:                "BarA",
		LastName:                 "Shared",
	}); err != nil {
		t.Fatal(err)
	}
	if flags := waitPeerFlags(t, ctx, c, aSelf.ID, aHash, false); flags.GetAddContact() || flags.GetReportSpam() || flags.GetBlockContact() {
		t.Fatalf("bar remained after addContact %#v", flags)
	}
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return userPhone(t, ctx, a, cID, cHash) != "", nil
	}); err != nil {
		t.Fatalf("shared phone: %v", err)
	}
	ids, err := contactIDList(ctx, c, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !containsInt32(ids, int32(aSelf.ID)) {
		t.Fatalf("getContactIDs %#v missing %d", ids, aSelf.ID)
	}
	saved, book := contactBook(t, ctx, c)
	hash := androidContactsHash(saved, book)
	same, err := contactIDList(ctx, c, hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(same) != 0 {
		t.Fatalf("matching hash returned %#v", same)
	}
}

func TestLeftChannelAppearsAfterLeave(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "bar_a", "BarA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "bar_b", "BarB")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	phone, err := cfg.Phone("bar_b")
	if err != nil {
		t.Fatal(err)
	}
	bID, bHash, err := a.ImportPhone(ctx, phone.Number, "BarB")
	if err != nil {
		t.Fatal(err)
	}
	chID, chHash, err := a.CreateMegagroup(ctx, "bar-left")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.InviteToChannel(ctx, inputChannel(chID, chHash), inputUser(bID, bHash)); err != nil {
		t.Fatal(err)
	}
	var memberHash int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hash, ok, err := b.ChatAccess(ctx, chID)
		if err != nil || !ok {
			return false, err
		}
		memberHash = hash
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Invoke(ctx, &mtproto.TLChannelsLeaveChannel{
		Constructor: mtproto.CRC32_channels_leaveChannel,
		Channel:     inputChannel(chID, memberHash),
	}); err != nil {
		t.Fatal(err)
	}
	chats, err := leftChannels(ctx, b, 0)
	if err != nil {
		t.Fatal(err)
	}
	var found *mtproto.Chat
	for _, chat := range chats {
		if chat.GetId() == chID {
			found = chat
		}
	}
	if found == nil || found.GetPredicateName() != mtproto.Predicate_channel || !found.GetLeft() {
		t.Fatalf("left channel %+v among %d", found, len(chats))
	}
}

func TestPersonalChannelOnFullUser(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "bar_a", "BarA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "bar_b", "BarB")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	created, err := a.Invoke(ctx, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Broadcast:   true,
		Title:       "bar-personal",
	})
	if err != nil {
		t.Fatal(err)
	}
	var chID, chHash int64
	for _, chat := range chatsOf(created) {
		if chat != nil && chat.GetBroadcast() {
			chID = chat.GetId()
			chHash = chatAccessHash(chat)
		}
	}
	if chID == 0 {
		t.Fatalf("createChannel %T", created)
	}
	msgID, err := a.SendText(ctx, inputPeerChannel(chID, chHash), "pinned bio", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLAccountUpdatePersonalChannel{
		Constructor: mtproto.CRC32_account_updatePersonalChannel,
		Channel:     inputChannel(chID, chHash),
	}); err != nil {
		t.Fatal(err)
	}
	fullObj, err := a.Invoke(ctx, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUserSelf(),
	})
	if err != nil {
		t.Fatal(err)
	}
	full := fullUserOf(t, fullObj)
	if full.GetPersonalChannelId().GetValue() != chID || full.GetPersonalChannelMessage().GetValue() != msgID {
		t.Fatalf("personal channel id %d message %d", full.GetPersonalChannelId().GetValue(), full.GetPersonalChannelMessage().GetValue())
	}
	if !chatListed(fullChats(fullObj), chID) {
		t.Fatal("personal channel missing from chats")
	}
	otherObj, err := b.Invoke(ctx, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUser(aSelf.ID, aSelf.AccessHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	other := fullUserOf(t, otherObj)
	if other.GetPersonalChannelId().GetValue() != 0 || other.GetPersonalChannelMessage() != nil {
		t.Fatalf("private personal channel leaked id %d", other.GetPersonalChannelId().GetValue())
	}
}

func setPhoneNobody(ctx context.Context, c *Client) error {
	_, err := c.Invoke(ctx, &mtproto.TLAccountSetPrivacy{
		Constructor: mtproto.CRC32_account_setPrivacy,
		Key:         mtproto.MakeTLInputPrivacyKeyPhoneNumber(&mtproto.InputPrivacyKey{}).To_InputPrivacyKey(),
		Rules: []*mtproto.InputPrivacyRule{
			mtproto.MakeTLInputPrivacyValueDisallowAll(&mtproto.InputPrivacyRule{}).To_InputPrivacyRule(),
		},
	})
	return err
}

func waitUserHash(t *testing.T, ctx context.Context, c *Client, id int64) int64 {
	t.Helper()
	var hash int64
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		page, err := c.dialogPage(ctx)
		if err != nil {
			return false, err
		}
		user := userByID(page.Users, id)
		if user == nil {
			return false, nil
		}
		hash = user.GetAccessHash().GetValue()
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	return hash
}

func waitPeerFlags(t *testing.T, ctx context.Context, c *Client, id, hash int64, want bool) *mtproto.PeerSettings {
	t.Helper()
	var got *mtproto.PeerSettings
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		settings, err := loadPeerSettings(ctx, c, id, hash)
		if err != nil {
			return false, err
		}
		got = settings
		if settings == nil {
			return !want, nil
		}
		return settings.GetAddContact() == want, nil
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

func loadPeerSettings(ctx context.Context, c *Client, id, hash int64) (*mtproto.PeerSettings, error) {
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetPeerSettings{
		Constructor: mtproto.CRC32_messages_getPeerSettings,
		Peer:        inputPeerUser(id, hash),
	})
	if err != nil {
		return nil, err
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesPeerSettings:
		return v.GetSettings(), nil
	case *mtproto.Messages_PeerSettings:
		return v.GetSettings(), nil
	default:
		return nil, fmt.Errorf("messages.getPeerSettings returned %T", obj)
	}
}

func userPhone(t *testing.T, ctx context.Context, c *Client, id, hash int64) string {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id:          []*mtproto.InputUser{inputUser(id, hash)},
	})
	if err != nil {
		t.Fatal(err)
	}
	user := userByID(usersOf(obj), id)
	if user == nil || user.GetPhone() == nil {
		return ""
	}
	return user.GetPhone().GetValue()
}

func contactIDList(ctx context.Context, c *Client, hash int64) ([]int32, error) {
	obj, err := c.Invoke(ctx, &mtproto.TLContactsGetContactIDs{
		Constructor: mtproto.CRC32_contacts_getContactIDs,
		Hash:        hash,
	})
	if err != nil {
		return nil, err
	}
	vec, ok := obj.(*tlVector)
	if !ok {
		return nil, fmt.Errorf("contacts.getContactIDs returned %T", obj)
	}
	return vec.Ints, nil
}

func contactBook(t *testing.T, ctx context.Context, c *Client) (int32, []int64) {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLContactsGetContacts{
		Constructor: mtproto.CRC32_contacts_getContacts,
	})
	if err != nil {
		t.Fatal(err)
	}
	book, ok := obj.(*mtproto.TLContactsContacts)
	if !ok {
		t.Fatalf("getContacts %T", obj)
	}
	ids := make([]int64, 0, len(book.GetContacts()))
	for _, row := range book.GetContacts() {
		if row.GetUserId() != 0 {
			ids = append(ids, row.GetUserId())
		}
	}
	return book.GetSavedCount(), ids
}

func leftChannels(ctx context.Context, c *Client, offset int32) ([]*mtproto.Chat, error) {
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetLeftChannels{
		Constructor: mtproto.CRC32_channels_getLeftChannels,
		Offset:      offset,
	})
	if err != nil {
		return nil, err
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesChats:
		return v.GetChats(), nil
	case *mtproto.TLMessagesChatsSlice:
		return v.GetChats(), nil
	default:
		return nil, fmt.Errorf("channels.getLeftChannels returned %T", obj)
	}
}

func fullChats(obj mtproto.TLObject) []*mtproto.Chat {
	switch v := obj.(type) {
	case *mtproto.TLUsersUserFull:
		return v.GetChats()
	case *mtproto.Users_UserFull:
		return v.GetChats()
	default:
		return nil
	}
}

func chatListed(chats []*mtproto.Chat, id int64) bool {
	for _, chat := range chats {
		if chat.GetId() == id {
			return true
		}
	}
	return false
}

func containsInt32(ids []int32, id int32) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func androidContactsHash(saved int32, ids []int64) int64 {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	acc := mixAndroidHash(0, int64(saved))
	for _, id := range ids {
		acc = mixAndroidHash(acc, id)
	}
	return acc
}

func mixAndroidHash(acc, n int64) int64 {
	u := uint64(acc)
	u ^= u >> 21
	u ^= u << 35
	u ^= u >> 4
	return int64(u + uint64(n))
}
