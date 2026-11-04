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
