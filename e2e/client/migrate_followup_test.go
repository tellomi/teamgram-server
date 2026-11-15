package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMigrateNoticeRightsAndOldInvite covers rows 189, 217, and 218.
func TestMigrateNoticeRightsAndOldInvite(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610709", "MigOwner")
	member, memberSelf := dialPhone(t, ctx, cfg, "9996610710", "MigMember")
	guest, _ := dialPhone(t, ctx, cfg, "9996610711", "MigGuest")

	chatID, err := owner.CreateChat(ctx, "move room", inputUser(memberSelf.ID, memberSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChat(chatID)
	if _, err = owner.Invoke(ctx, &mtproto.TLMessagesEditChatDefaultBannedRights{
		Constructor: mtproto.CRC32_messages_editChatDefaultBannedRights,
		Peer:        peer,
		BannedRights: mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
			SendMedia:  true,
			SendPhotos: true,
		}).To_ChatBannedRights(),
	}); err != nil {
		t.Fatal(err)
	}
	link := exportInvite(t, ctx, owner, &mtproto.TLMessagesExportChatInvite{
		Constructor:           mtproto.CRC32_messages_exportChatInvite_a455de90,
		LegacyRevokePermanent: true,
		Peer:                  peer,
	})
	hash := strings.TrimPrefix(link.GetLink(), "https://tell.cc/+")
	if hash == link.GetLink() {
		hash = strings.TrimPrefix(link.GetLink(), "http://tell.cc/+")
	}

	up, err := owner.Invoke(ctx, &mtproto.TLMessagesMigrateChat{
		Constructor: mtproto.CRC32_messages_migrateChat,
		ChatId:      chatID,
	})
	if err != nil {
		t.Fatal(err)
	}
	old, channelID, channelHash := migratedChat(t, up, chatID)
	if !old.GetDeactivated() || old.GetMigratedTo().GetChannelId() == 0 {
		t.Fatalf("old chat %#v", old)
	}
	if !historyHasMigrate(t, owner, chatID) {
		t.Fatal("owner history has no ChatMigrateTo")
	}
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		return historyHasMigrate(t, member, chatID), nil
	}); err != nil {
		t.Fatal("member history has no ChatMigrateTo")
	}

	fullObj, err := owner.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     inputChannel(channelID, channelHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	var rights *mtproto.ChatBannedRights
	for _, ch := range chatsOf(fullObj) {
		if ch.GetId() == channelID {
			rights = ch.GetDefaultBannedRights()
		}
	}
	if !rights.GetSendMedia() || !rights.GetSendPhotos() {
		t.Fatalf("channel default banned %#v", rights)
	}

	previewObj, err := guest.Invoke(ctx, &mtproto.TLMessagesCheckChatInvite{
		Constructor: mtproto.CRC32_messages_checkChatInvite,
		Hash:        hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview := asChatInvite(t, previewObj)
	if !preview.GetMegagroup() || preview.GetTitle() != "move room" {
		t.Fatalf("preview %#v", preview)
	}
	if _, err = guest.Invoke(ctx, &mtproto.TLMessagesImportChatInvite6C50051C{
		Constructor: mtproto.CRC32_messages_importChatInvite6C50051C,
		Hash:        hash,
	}); err != nil {
		t.Fatal(err)
	}
}

func migratedChat(t *testing.T, obj mtproto.TLObject, chatID int64) (*mtproto.Chat, int64, int64) {
	t.Helper()
	var old *mtproto.Chat
	for _, ch := range chatsOf(obj) {
		if ch.GetId() == chatID && !ch.GetMegagroup() {
			old = ch
		}
	}
	if old == nil {
		t.Fatalf("migrate response has no old chat in %T", obj)
	}
	return old, old.GetMigratedTo().GetChannelId(), old.GetMigratedTo().GetAccessHash()
}

func historyHasMigrate(t *testing.T, c *Client, chatID int64) bool {
	t.Helper()
	ctx, cancel := contextTimeout(20 * time.Second)
	defer cancel()
	msgs, err := c.History(ctx, inputPeerChat(chatID), 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if m.GetAction().GetPredicateName() == mtproto.Predicate_messageActionChatMigrateTo {
			return true
		}
	}
	return false
}

func asChatInvite(t *testing.T, obj mtproto.TLObject) *mtproto.ChatInvite {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.ChatInvite:
		return v
	case *mtproto.TLChatInvite:
		return v.To_ChatInvite()
	case *mtproto.TLChatInviteAlready:
		return v.To_ChatInvite()
	default:
		t.Fatalf("invite %T", obj)
		return nil
	}
}
