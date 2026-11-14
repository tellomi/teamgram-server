package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestParticipantFiltersAndInviteSearch covers the member-list and join-request
// fixes: contacts and q filters, the banned-list admin check, hidden members
// still showing admins, and getChatInviteImporters name search with users.
// Phones 9996610471–9996610473.
func TestParticipantFiltersAndInviteSearch(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	owner, _ := dialPhone(t, ctx, cfg, "9996610471", "OwnerG")
	ada, adaSelf := dialPhone(t, ctx, cfg, "9996610472", "AdaSearch")
	bob, bobSelf := dialPhone(t, ctx, cfg, "9996610473", "BobOther")
	setProfileName(t, ctx, ada, "AdaSearch", "")
	setProfileName(t, ctx, bob, "BobOther", "")

	adaID, adaHash, err := owner.ImportPhone(ctx, "9996610472", "AdaSearch")
	if err != nil {
		t.Fatal(err)
	}
	bobID, bobHash, err := owner.ImportPhone(ctx, "9996610473", "BobOther")
	if err != nil {
		t.Fatal(err)
	}
	_ = adaSelf
	_ = bobSelf

	groupID, groupHash, err := owner.CreateMegagroup(ctx, "FilterRoom")
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(groupID, groupHash)
	if err = owner.InviteToChannel(ctx, channel, inputUser(adaID, adaHash), inputUser(bobID, bobHash)); err != nil {
		t.Fatal(err)
	}

	contacts := participantsOf(t, ctx, owner, channel, mtproto.MakeTLChannelParticipantsContacts(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter())
	if ids := participantUserIDs(contacts); !sameIDSet(ids, []int64{adaID, bobID}) {
		t.Fatalf("contacts ids=%v want %d and %d", ids, adaID, bobID)
	}
	search := participantsOf(t, ctx, owner, channel, mtproto.MakeTLChannelParticipantsSearch(&mtproto.ChannelParticipantsFilter{
		Q_STRING: "AdaSearch",
	}).To_ChannelParticipantsFilter())
	if ids := participantUserIDs(search); !sameIDSet(ids, []int64{adaID}) {
		t.Fatalf("search ids=%v want %d", ids, adaID)
	}

	var memberHash int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hash, ok, err := bob.ChatAccess(ctx, groupID)
		if err != nil || !ok {
			return false, err
		}
		memberHash = hash
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err = bob.Invoke(ctx, &mtproto.TLChannelsGetParticipants{
		Constructor: mtproto.CRC32_channels_getParticipants,
		Channel:     inputChannel(groupID, memberHash),
		Filter:      mtproto.MakeTLChannelParticipantsKicked(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter(),
		Limit:       50,
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member kicked list: %v", err)
	}

	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsToggleParticipantsHidden{
		Constructor: mtproto.CRC32_channels_toggleParticipantsHidden,
		Channel:     channel,
		Enabled:     mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	hidden := participantsOf(t, ctx, bob, inputChannel(groupID, memberHash), mtproto.MakeTLChannelParticipantsRecent(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter())
	if len(hidden.GetParticipants()) != 0 {
		t.Fatalf("hidden recent %v", participantUserIDs(hidden))
	}
	admins := participantsOf(t, ctx, bob, inputChannel(groupID, memberHash), mtproto.MakeTLChannelParticipantsAdmins(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter())
	if !hasParticipant(admins, ownerID(t, ctx, owner)) {
		t.Fatalf("hidden admins %v", participantUserIDs(admins))
	}
	fullObj, err := bob.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     inputChannel(groupID, memberHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	full := asChatFull(t, fullObj)
	if full.GetCanViewParticipants() || !full.GetParticipantsHidden() {
		t.Fatalf("can_view=%v hidden=%v", full.GetCanViewParticipants(), full.GetParticipantsHidden())
	}

	link := exportInvite(t, ctx, owner, &mtproto.TLMessagesExportChatInvite{
		Constructor:           mtproto.CRC32_messages_exportChatInvite_a455de90,
		LegacyRevokePermanent: true,
		Peer:                  inputPeerChannel(groupID, groupHash),
	})
	guest, _ := dialPhone(t, ctx, cfg, "9996610474", "CaraGuest")
	setProfileName(t, ctx, guest, "CaraGuest", "")
	if _, err = guest.Invoke(ctx, &mtproto.TLMessagesImportChatInviteDE91436E{
		Constructor: mtproto.CRC32_messages_importChatInviteDE91436E,
		Hash:        inviteHash(link.GetLink()),
	}); err != nil {
		t.Fatal(err)
	}
	found := inviteImporters(t, ctx, owner, inputPeerChannel(groupID, groupHash), "Cara")
	if found.GetCount() < 1 || len(found.GetUsers()) == 0 {
		t.Fatalf("importers %+v", found)
	}
	var saw bool
	for _, row := range found.GetImporters() {
		if row.GetUserId() == 0 {
			continue
		}
		for _, u := range found.GetUsers() {
			if u.GetId() == row.GetUserId() && u.GetFirstName().GetValue() == "CaraGuest" {
				saw = true
			}
		}
	}
	if !saw {
		t.Fatalf("search did not return CaraGuest users=%v importers=%v", found.GetUsers(), found.GetImporters())
	}
}

func setProfileName(t *testing.T, ctx context.Context, c *Client, first, last string) {
	t.Helper()
	if _, err := c.Invoke(ctx, &mtproto.TLAccountUpdateProfile{
		Constructor: mtproto.CRC32_account_updateProfile,
		FirstName:   strFlag(first),
		LastName:    strFlag(last),
	}); err != nil {
		t.Fatal(err)
	}
}

func participantsOf(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel, filter *mtproto.ChannelParticipantsFilter) *mtproto.Channels_ChannelParticipants {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetParticipants{
		Constructor: mtproto.CRC32_channels_getParticipants,
		Channel:     channel,
		Filter:      filter,
		Limit:       100,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLChannelsChannelParticipants:
		return v.To_Channels_ChannelParticipants()
	case *mtproto.Channels_ChannelParticipants:
		return v
	default:
		t.Fatalf("participants %T", obj)
		return nil
	}
}

func participantUserIDs(got *mtproto.Channels_ChannelParticipants) []int64 {
	out := make([]int64, 0, len(got.GetParticipants()))
	for _, p := range got.GetParticipants() {
		id := p.GetUserId()
		if id == 0 && p.GetPeer() != nil {
			id = p.GetPeer().GetUserId()
		}
		if id != 0 {
			out = append(out, id)
		}
	}
	return out
}

func hasParticipant(got *mtproto.Channels_ChannelParticipants, id int64) bool {
	for _, gotID := range participantUserIDs(got) {
		if gotID == id {
			return true
		}
	}
	return false
}

func sameIDSet(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[int64]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		if seen[id] == 0 {
			return false
		}
		seen[id]--
	}
	return true
}

func ownerID(t *testing.T, ctx context.Context, c *Client) int64 {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id:          []*mtproto.InputUser{inputUserSelf()},
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *tlVector:
		if len(v.Users) == 0 || v.Users[0].GetId() == 0 {
			t.Fatal("users.getUsers empty")
		}
		return v.Users[0].GetId()
	default:
		for _, u := range usersOf(obj) {
			if u != nil && u.GetId() != 0 {
				return u.GetId()
			}
		}
	}
	t.Fatalf("users.getUsers %T", obj)
	return 0
}

func inviteHash(link string) string {
	const mark = "/+"
	i := len(link)
	if n := len(link); n > 0 {
		for j := 0; j+1 < n; j++ {
			if link[j:j+2] == mark {
				i = j + 2
			}
		}
	}
	if i >= len(link) {
		return link
	}
	return link[i:]
}

func inviteImporters(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, q string) *mtproto.Messages_ChatInviteImporters {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetChatInviteImporters{
		Constructor: mtproto.CRC32_messages_getChatInviteImporters,
		Peer:        peer,
		Q:           &wrapperspb.StringValue{Value: q},
		OffsetUser:  mtproto.MakeTLInputUserEmpty(nil).To_InputUser(),
		Limit:       20,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesChatInviteImporters:
		return v.To_Messages_ChatInviteImporters()
	case *mtproto.Messages_ChatInviteImporters:
		return v
	default:
		t.Fatalf("importers %T", obj)
		return nil
	}
}
