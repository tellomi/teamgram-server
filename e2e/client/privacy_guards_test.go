package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Phones 9996610461 and 9996610462 are this task's range.

func TestGiftPrivacyPersists(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610461", "Gifts")
	settings := mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{
		DisplayGiftsButton: true,
		DisallowedGifts: mtproto.MakeTLDisallowedGiftsSettings(&mtproto.DisallowedGiftsSettings{
			DisallowPremiumGifts: true,
		}).To_DisallowedGiftsSettings(),
		NoncontactPeersPaidStars: wrapperspb.Int64(5),
	}).To_GlobalPrivacySettings()
	obj, err := c.Invoke(ctx, &mtproto.TLAccountSetGlobalPrivacySettings{
		Constructor: mtproto.CRC32_account_setGlobalPrivacySettings,
		Settings:    settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := privacyOf(t, obj)
	if !got.GetDisplayGiftsButton() || !got.GetDisallowedGifts().GetDisallowPremiumGifts() || got.GetNoncontactPeersPaidStars().GetValue() != 5 {
		t.Fatalf("set reply %+v", got)
	}
	again, err := c.Invoke(ctx, &mtproto.TLAccountGetGlobalPrivacySettings{
		Constructor: mtproto.CRC32_account_getGlobalPrivacySettings,
	})
	if err != nil {
		t.Fatal(err)
	}
	got = privacyOf(t, again)
	if !got.GetDisplayGiftsButton() || !got.GetDisallowedGifts().GetDisallowPremiumGifts() || got.GetNoncontactPeersPaidStars().GetValue() != 5 {
		t.Fatalf("get %+v", got)
	}
}

func TestSponsoredTogglePersists(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, self := dialPhone(t, ctx, cfg, "9996610461", "Sponsored")
	if _, err := c.Invoke(ctx, &mtproto.TLAccountToggleSponsoredMessages{
		Constructor: mtproto.CRC32_account_toggleSponsoredMessages,
		Enabled:     mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	full, err := c.Invoke(ctx, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUser(self.ID, self.AccessHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fullUserOf(t, full).GetSponsoredEnabled() {
		t.Fatal("sponsored_enabled stayed false")
	}
}

func TestToggleUsernameStopsResolve(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610461", "Uname")
	other, _ := dialPhone(t, ctx, cfg, "9996610462", "UnamePeer")
	name := randomUsername()
	if _, err := owner.Invoke(ctx, &mtproto.TLAccountUpdateUsername{
		Constructor: mtproto.CRC32_account_updateUsername,
		Username:    name,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Invoke(ctx, &mtproto.TLAccountToggleUsername{
		Constructor: mtproto.CRC32_account_toggleUsername,
		Username:    name,
		Active:      mtproto.BoolFalse,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := other.Invoke(ctx, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_f93ccba3,
		Username:    name,
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "USERNAME_NOT_OCCUPIED" {
		t.Fatalf("resolve after toggle: %v", err)
	}
}
