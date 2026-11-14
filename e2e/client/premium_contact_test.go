package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestNoncontactPremiumRequired follows globalPrivacySettings.new_noncontact_peers_require_premium.
// A stranger sees user.contact_require_premium and userFull.contact_require_premium,
// users.getRequirementsToContact returns requirementToContactPremium, and
// messages.sendMessage returns 403 PRIVACY_PREMIUM_REQUIRED.
// A contact, an existing private chat, and an unexpired users.premium row can write.
// TELLOMI_PREMIUM_EVERYONE does not waive the gate.
func TestNoncontactPremiumRequired(t *testing.T) {
	cfg := requireE2E(t)
	owner, ownerSelf := loginNamed(t, cfg, ProfileAndroid, "gpriv_owner", "GprivOwner")
	stranger, _ := loginNamed(t, cfg, ProfileIOS, "gpriv_stranger", "GprivStranger")
	member, memberSelf := loginNamed(t, cfg, ProfileAndroid, "gpriv_member", "GprivMember")
	reply, replySelf := loginNamed(t, cfg, ProfileIOS, "gpriv_reply", "GprivReply")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	prev := getPrivacy(t, ctx, owner)
	t.Cleanup(func() {
		cctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		setNoncontactPremium(t, cctx, owner, prev, false)
		mustSQL(t, fmt.Sprintf(`UPDATE users SET premium = 0, premium_expire_date = 0 WHERE id = %d`, memberSelf.ID))
	})
	setNoncontactPremium(t, ctx, owner, prev, true)

	ownerPeer := inputPeerUser(ownerSelf.ID, ownerSelf.AccessHash)
	full := fullUserOf(t, invokeOK(t, stranger, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUser(ownerSelf.ID, ownerSelf.AccessHash),
	}))
	if !full.GetContactRequirePremium() {
		t.Fatal("userFull.contact_require_premium")
	}
	card := userByID(usersOf(invokeOK(t, stranger, &mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id:          []*mtproto.InputUser{inputUser(ownerSelf.ID, ownerSelf.AccessHash)},
	})), ownerSelf.ID)
	if card == nil || !card.GetContactRequirePremium() {
		t.Fatal("user.contact_require_premium")
	}
	if names := requirementNames(t, stranger, ownerSelf); len(names) != 1 || names[0] != mtproto.Predicate_requirementToContactPremium {
		t.Fatalf("requirements %v", names)
	}
	if names := premiumRequiredNames(t, stranger, ownerSelf); len(names) != 1 || names[0] != mtproto.Predicate_boolTrue {
		t.Fatalf("is premium required %v", names)
	}
	_, err := stranger.SendText(ctx, ownerPeer, "blocked-"+fmt.Sprint(time.Now().UnixNano()), nil)
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 403 || rpc.Message != "PRIVACY_PREMIUM_REQUIRED" {
		t.Fatalf("stranger send: %v", err)
	}

	ownerPhone, err := cfg.Phone("gpriv_stranger")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLContactsImportContacts{
		Constructor: mtproto.CRC32_contacts_importContacts,
		Contacts: []*mtproto.InputContact{
			mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{
				ClientId:  1,
				Phone:     ownerPhone.Number,
				FirstName: "GprivStranger",
			}).To_InputContact(),
		},
	}); err != nil {
		t.Fatal(err)
	}
	full = fullUserOf(t, invokeOK(t, stranger, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUser(ownerSelf.ID, ownerSelf.AccessHash),
	}))
	if full.GetContactRequirePremium() {
		t.Fatal("contact still blocked on userFull")
	}
	if card = userByID(usersOf(invokeOK(t, stranger, &mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id:          []*mtproto.InputUser{inputUser(ownerSelf.ID, ownerSelf.AccessHash)},
	})), ownerSelf.ID); card == nil || !card.GetContactRequirePremium() {
		t.Fatal("contact lost the raw user flag")
	}
	if _, err = stranger.SendText(ctx, ownerPeer, "contact-ok-"+fmt.Sprint(time.Now().UnixNano()), nil); err != nil {
		t.Fatalf("contact send: %v", err)
	}

	// An existing private chat is the third exemption. The owner writes first.
	if _, err = owner.SendText(ctx, inputPeerUser(replySelf.ID, replySelf.AccessHash), "opened-"+fmt.Sprint(time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	full = fullUserOf(t, invokeOK(t, reply, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUser(ownerSelf.ID, ownerSelf.AccessHash),
	}))
	if full.GetContactRequirePremium() {
		t.Fatal("existing private chat still blocked on userFull")
	}
	if _, err = reply.SendText(ctx, ownerPeer, "reply-ok-"+fmt.Sprint(time.Now().UnixNano()), nil); err != nil {
		t.Fatalf("existing chat send: %v", err)
	}

	_, err = member.SendText(ctx, ownerPeer, "member-blocked-"+fmt.Sprint(time.Now().UnixNano()), nil)
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 403 || rpc.Message != "PRIVACY_PREMIUM_REQUIRED" {
		t.Fatalf("member before premium: %v", err)
	}
	mustSQL(t, fmt.Sprintf(`UPDATE users SET premium = 1, premium_expire_date = %d WHERE id = %d`, time.Now().Unix()+30*86400, memberSelf.ID))
	invokeOK(t, member, &mtproto.TLAccountUpdateProfile{
		Constructor: mtproto.CRC32_account_updateProfile,
		FirstName:   wrapperspb.String("GprivMember"),
	})
	full = fullUserOf(t, invokeOK(t, member, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUser(ownerSelf.ID, ownerSelf.AccessHash),
	}))
	if full.GetContactRequirePremium() {
		t.Fatal("premium subscriber still blocked on userFull")
	}
	if _, err = member.SendText(ctx, ownerPeer, "member-ok-"+fmt.Sprint(time.Now().UnixNano()), nil); err != nil {
		t.Fatalf("premium send: %v", err)
	}
}

func setNoncontactPremium(t *testing.T, ctx context.Context, c *Client, base *mtproto.GlobalPrivacySettings, require bool) {
	t.Helper()
	settings := mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{
		ArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN: base.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN(),
		KeepArchivedUnmuted:                          base.GetKeepArchivedUnmuted(),
		KeepArchivedFolders:                          base.GetKeepArchivedFolders(),
		HideReadMarks:                                base.GetHideReadMarks(),
		NewNoncontactPeersRequirePremium:             require,
	}).To_GlobalPrivacySettings()
	obj, err := c.Invoke(ctx, &mtproto.TLAccountSetGlobalPrivacySettings{
		Constructor: mtproto.CRC32_account_setGlobalPrivacySettings,
		Settings:    settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	if privacyOf(t, obj).GetNewNoncontactPeersRequirePremium() != require {
		t.Fatalf("stored %+v", privacyOf(t, obj))
	}
}

func requirementNames(t *testing.T, c *Client, self Self) []string {
	t.Helper()
	obj := invokeOK(t, c, &mtproto.TLUsersGetRequirementsToContact{
		Constructor: mtproto.CRC32_users_getRequirementsToContact,
		Id:          []*mtproto.InputUser{inputUser(self.ID, self.AccessHash)},
	})
	vec, ok := obj.(*tlVector)
	if !ok {
		t.Fatalf("requirements %T", obj)
	}
	return vec.Names
}

func premiumRequiredNames(t *testing.T, c *Client, self Self) []string {
	t.Helper()
	obj := invokeOK(t, c, &mtproto.TLUsersGetIsPremiumRequiredToContact{
		Constructor: mtproto.CRC32_users_getIsPremiumRequiredToContact,
		Id:          []*mtproto.InputUser{inputUser(self.ID, self.AccessHash)},
	})
	vec, ok := obj.(*tlVector)
	if !ok {
		t.Fatalf("premium required %T", obj)
	}
	return vec.Names
}
