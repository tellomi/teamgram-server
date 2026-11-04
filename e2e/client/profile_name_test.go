package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestProfileAndChannelNames(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileIOS, "username", "User")
	t.Cleanup(func() {
		cctx, cancel := contextTimeout(20 * time.Second)
		defer cancel()
		_, _ = c.Invoke(cctx, &mtproto.TLAccountUpdateProfile{
			Constructor: mtproto.CRC32_account_updateProfile,
			FirstName:   strFlag("User"),
			LastName:    strFlag(""),
			About:       strFlag(""),
		})
	})

	cctx, cancel := contextTimeout(20 * time.Second)
	_, err := c.Invoke(cctx, &mtproto.TLAccountUpdateProfile{
		Constructor: mtproto.CRC32_account_updateProfile,
		FirstName:   strFlag("admin"),
	})
	cancel()
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "FIRSTNAME_INVALID" {
		t.Fatalf("first name admin: %v", err)
	}

	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.Invoke(cctx, &mtproto.TLAccountUpdateProfile{
		Constructor: mtproto.CRC32_account_updateProfile,
		LastName:    strFlag("tellomi"),
	})
	cancel()
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "LASTNAME_INVALID" {
		t.Fatalf("last name tellomi: %v", err)
	}

	cctx, cancel = contextTimeout(20 * time.Second)
	updated, err := c.Invoke(cctx, &mtproto.TLAccountUpdateProfile{
		Constructor: mtproto.CRC32_account_updateProfile,
		FirstName:   strFlag("Ada"),
		LastName:    strFlag("Lovelace"),
	})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	user, ok := updated.(*mtproto.TLUser)
	if !ok {
		t.Fatalf("account.updateProfile returned %T", updated)
	}
	if got := user.To_User().GetFirstName().GetValue(); got != "Ada" {
		t.Fatalf("first name = %q", got)
	}
	if got := user.To_User().GetLastName().GetValue(); got != "Lovelace" {
		t.Fatalf("last name = %q", got)
	}

	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.Invoke(cctx, &mtproto.TLAccountUpdateProfile{
		Constructor: mtproto.CRC32_account_updateProfile,
		About:       strFlag("admin"),
	})
	cancel()
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "FIRSTNAME_INVALID" {
		t.Fatalf("about admin: %v", err)
	}

	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.Invoke(cctx, &mtproto.TLAccountUpdateProfile{
		Constructor: mtproto.CRC32_account_updateProfile,
		About:       strFlag("Reads books"),
	})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	cctx, cancel = contextTimeout(20 * time.Second)
	fullUser, err := c.Invoke(cctx, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUserSelf(),
	})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if got := userAbout(t, fullUser); got != "Reads books" {
		t.Fatalf("about = %q", got)
	}

	cctx, cancel = contextTimeout(20 * time.Second)
	_, _, err = c.CreateMegagroup(cctx, "admin")
	cancel()
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "FIRSTNAME_INVALID" {
		t.Fatalf("megagroup admin: %v", err)
	}

	cctx, cancel = contextTimeout(20 * time.Second)
	id, hash, err := c.CreateMegagroup(cctx, "Book Club")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.Invoke(cctx, &mtproto.TLChannelsEditTitle{
		Constructor: mtproto.CRC32_channels_editTitle,
		Channel:     inputChannel(id, hash),
		Title:       "tellomi",
	})
	cancel()
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "FIRSTNAME_INVALID" {
		t.Fatalf("edit title: %v", err)
	}

	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.Invoke(cctx, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Megagroup:   true,
		Title:       "Book Club",
		About:       "tellomi",
	})
	cancel()
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "FIRSTNAME_INVALID" {
		t.Fatalf("megagroup about: %v", err)
	}

	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.Invoke(cctx, &mtproto.TLMessagesEditChatAbout{
		Constructor: mtproto.CRC32_messages_editChatAbout,
		Peer:        inputPeerChannel(id, hash),
		About:       "admin",
	})
	cancel()
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "FIRSTNAME_INVALID" {
		t.Fatalf("edit megagroup about: %v", err)
	}
	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.Invoke(cctx, &mtproto.TLMessagesEditChatAbout{
		Constructor: mtproto.CRC32_messages_editChatAbout,
		Peer:        inputPeerChannel(id, hash),
		About:       "club notes",
	})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	cctx, cancel = contextTimeout(20 * time.Second)
	fullChannel, err := c.Invoke(cctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     inputChannel(id, hash),
	})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if got := asChatFull(t, fullChannel).GetAbout(); got != "club notes" {
		t.Fatalf("megagroup about = %q", got)
	}

	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.CreateChat(cctx, "admin")
	cancel()
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "FIRSTNAME_INVALID" {
		t.Fatalf("basic group admin: %v", err)
	}
	cctx, cancel = contextTimeout(20 * time.Second)
	chatID, err := c.CreateChat(cctx, "读书会")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.Invoke(cctx, &mtproto.TLMessagesEditChatTitle{
		Constructor: mtproto.CRC32_messages_editChatTitle,
		ChatId:      chatID,
		Title:       "tellomi",
	})
	cancel()
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "FIRSTNAME_INVALID" {
		t.Fatalf("basic group title: %v", err)
	}
	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.Invoke(cctx, &mtproto.TLMessagesEditChatAbout{
		Constructor: mtproto.CRC32_messages_editChatAbout,
		Peer:        inputPeerChat(chatID),
		About:       "admin",
	})
	cancel()
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "FIRSTNAME_INVALID" {
		t.Fatalf("basic group about: %v", err)
	}
	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.Invoke(cctx, &mtproto.TLMessagesEditChatAbout{
		Constructor: mtproto.CRC32_messages_editChatAbout,
		Peer:        inputPeerChat(chatID),
		About:       "weekly notes",
	})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	cctx, cancel = contextTimeout(20 * time.Second)
	fullChat, err := c.Invoke(cctx, &mtproto.TLMessagesGetFullChat{
		Constructor: mtproto.CRC32_messages_getFullChat,
		ChatId:      chatID,
	})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if got := asChatFull(t, fullChat).GetAbout(); got != "weekly notes" {
		t.Fatalf("basic group about = %q", got)
	}

	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.SendText(cctx, inputPeerChannel(id, hash), "admin tellomi", nil)
	cancel()
	if err != nil {
		t.Fatalf("chat text must not be filtered: %v", err)
	}
	cctx, cancel = contextTimeout(20 * time.Second)
	_, err = c.SendText(cctx, inputPeerSelf(), "admin tellomi", nil)
	cancel()
	if err != nil {
		t.Fatalf("saved message must not be filtered: %v", err)
	}
}

func userAbout(t *testing.T, obj mtproto.TLObject) string {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLUsersUserFull:
		return v.To_Users_UserFull().GetFullUser().GetAbout().GetValue()
	case *mtproto.Users_UserFull:
		return v.GetFullUser().GetAbout().GetValue()
	default:
		t.Fatalf("users.getFullUser returned %T", obj)
		return ""
	}
}
