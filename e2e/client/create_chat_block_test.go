package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestCreateChatSkipsBlockedAndPrivateMembers is the production regression
// where messages.createChat still inserted a user who had blocked the creator.
// Layer 232 returns messages.invitedUsers and lists that user in
// missing_invitees. The older Updates result is USER_PRIVACY_RESTRICTED when
// nobody else can be added. channels.createChannel has no member list.
func TestCreateChatSkipsBlockedAndPrivateMembers(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "crblk_a", "BlkA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "crblk_b", "BlkB")
	_, okSelf := loginNamed(t, cfg, ProfileAndroid, "crblk_ok", "BlkOK")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if err := b.blockUser(ctx, aSelf); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.unblockUser(ctx, aSelf) }()

	_, err := a.CreateChat(ctx, "legacy-blocked", inputUser(bSelf.ID, bSelf.AccessHash))
	if err == nil || !strings.Contains(err.Error(), "USER_PRIVACY_RESTRICTED") {
		t.Fatalf("legacy createChat with the blocker: %v", err)
	}

	invited := createChatLayer232(t, ctx, a, "layer-blocked", inputUser(bSelf.ID, bSelf.AccessHash), inputUser(okSelf.ID, okSelf.AccessHash))
	if !missingHas(invited.GetMissingInvitees(), bSelf.ID) {
		t.Fatalf("missing_invitees %+v", invited.GetMissingInvitees())
	}
	chatID := basicChatID(t, invited.GetUpdates(), "layer-blocked")
	if participantIDs(t, ctx, a, chatID)[bSelf.ID] {
		t.Fatal("blocked user was inserted into the new group")
	}
	if !participantIDs(t, ctx, a, chatID)[okSelf.ID] {
		t.Fatal("an allowed member was not inserted")
	}

	if err = b.unblockUser(ctx, aSelf); err != nil {
		t.Fatal(err)
	}
	if err = b.setChatInviteNobody(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.setChatInviteEveryone(ctx) }()

	_, err = a.CreateChat(ctx, "legacy-privacy", inputUser(bSelf.ID, bSelf.AccessHash))
	if err == nil || !strings.Contains(err.Error(), "USER_PRIVACY_RESTRICTED") {
		t.Fatalf("legacy createChat against chat-invite privacy: %v", err)
	}
	private := createChatLayer232(t, ctx, a, "layer-privacy", inputUser(bSelf.ID, bSelf.AccessHash))
	if !missingHas(private.GetMissingInvitees(), bSelf.ID) {
		t.Fatalf("privacy missing %+v", private.GetMissingInvitees())
	}

	if err = b.setChatInviteEveryone(ctx); err != nil {
		t.Fatal(err)
	}
	openID, err := a.CreateChat(ctx, "legacy-open", inputUser(bSelf.ID, bSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	if !participantIDs(t, ctx, a, openID)[bSelf.ID] {
		t.Fatal("allowed member was skipped")
	}
}

func (c *Client) blockUser(ctx context.Context, peer Self) error {
	_, err := c.Invoke(ctx, &mtproto.TLContactsBlock{
		Constructor: mtproto.CRC32_contacts_block_2e2e8734,
		Id:          inputPeerUser(peer.ID, peer.AccessHash),
	})
	return err
}

func (c *Client) unblockUser(ctx context.Context, peer Self) error {
	_, err := c.Invoke(ctx, &mtproto.TLContactsUnblock{
		Constructor: mtproto.CRC32_contacts_unblock_b550d328,
		Id:          inputPeerUser(peer.ID, peer.AccessHash),
	})
	return err
}

func (c *Client) setChatInviteNobody(ctx context.Context) error {
	return c.setChatInvite(ctx, mtproto.MakeTLInputPrivacyValueDisallowAll(&mtproto.InputPrivacyRule{}).To_InputPrivacyRule())
}

func (c *Client) setChatInviteEveryone(ctx context.Context) error {
	return c.setChatInvite(ctx, mtproto.MakeTLInputPrivacyValueAllowAll(&mtproto.InputPrivacyRule{}).To_InputPrivacyRule())
}

func (c *Client) setChatInvite(ctx context.Context, rule *mtproto.InputPrivacyRule) error {
	_, err := c.Invoke(ctx, &mtproto.TLAccountSetPrivacy{
		Constructor: mtproto.CRC32_account_setPrivacy,
		Key:         mtproto.MakeTLInputPrivacyKeyChatInvite(&mtproto.InputPrivacyKey{}).To_InputPrivacyKey(),
		Rules:       []*mtproto.InputPrivacyRule{rule},
	})
	return err
}

func createChatLayer232(t *testing.T, ctx context.Context, c *Client, title string, users ...*mtproto.InputUser) *mtproto.Messages_InvitedUsers {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesCreateChat92CEDDD4{
		Constructor: mtproto.CRC32_messages_createChat92CEDDD4,
		Users:       users,
		Title:       title,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch res := obj.(type) {
	case *mtproto.Messages_InvitedUsers:
		return res
	case *mtproto.TLMessagesInvitedUsers:
		return res.To_Messages_InvitedUsers()
	default:
		t.Fatalf("createChat layer 232 returned %T", obj)
		return nil
	}
}

func missingHas(rows []*mtproto.MissingInvitee, id int64) bool {
	for _, row := range rows {
		if row.GetUserId() == id {
			return true
		}
	}
	return false
}

func basicChatID(t *testing.T, updates *mtproto.Updates, title string) int64 {
	t.Helper()
	for _, chat := range chatsOf(updatesObject(updates)) {
		if chat == nil || chat.GetMegagroup() || chat.GetBroadcast() || chat.GetId() == 0 {
			continue
		}
		if title == "" || chat.GetTitle() == title || chat.GetPredicateName() == mtproto.Predicate_chat {
			return chat.GetId()
		}
	}
	t.Fatalf("no basic group in %T", updates)
	return 0
}

func participantIDs(t *testing.T, ctx context.Context, c *Client, chatID int64) map[int64]bool {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetFullChat{
		Constructor: mtproto.CRC32_messages_getFullChat,
		ChatId:      chatID,
	})
	if err != nil {
		t.Fatal(err)
	}
	full := asChatFull(t, obj)
	out := map[int64]bool{}
	parts := full.GetParticipants()
	if parts == nil {
		return out
	}
	for _, p := range parts.GetParticipants() {
		if p.GetUserId() != 0 {
			out[p.GetUserId()] = true
		}
	}
	return out
}
