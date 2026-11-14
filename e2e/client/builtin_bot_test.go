package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestBuiltinBotsResolveAndOpenProfile is contacts.resolveUsername and
// users.getFullUser for the reserved bots that have no users-table row.
// The name match is case-insensitive. A name nobody holds does not come
// back as a peer.
func TestBuiltinBotsResolveAndOpenProfile(t *testing.T) {
	cfg := requireE2E(t)
	phone, err := cfg.Phone("bot_resolve")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, phone.Number, "BotResolve")

	for _, name := range []string{"BotFather", "botfather", "bing", "GIF", "gif", "venue"} {
		res, err := c.Invoke(ctx, &mtproto.TLContactsResolveUsername{
			Constructor: mtproto.CRC32_contacts_resolveUsername_725afbbc,
			Username:    name,
		})
		if err != nil {
			t.Fatalf("resolve %s: %v", name, err)
		}
		peer, ok := res.(*mtproto.TLContactsResolvedPeer)
		if !ok || peer.GetPeer().GetUserId() == 0 {
			t.Fatalf("resolve %s returned %T", name, res)
		}
		id := peer.GetPeer().GetUserId()
		var bot *mtproto.User
		for _, u := range peer.GetUsers() {
			if u.GetId() == id {
				bot = u
			}
		}
		if bot == nil || !bot.GetBot() || bot.GetAccessHash().GetValue() == 0 {
			t.Fatalf("resolve %s users missing bot id %d", name, id)
		}
		full, err := c.Invoke(ctx, &mtproto.TLUsersGetFullUser{
			Constructor: mtproto.CRC32_users_getFullUser,
			Id:          inputUser(id, bot.GetAccessHash().GetValue()),
		})
		if err != nil {
			t.Fatalf("getFullUser %s: %v", name, err)
		}
		opened, ok := full.(*mtproto.TLUsersUserFull)
		if !ok {
			t.Fatalf("getFullUser %s returned %T", name, full)
		}
		if opened.GetFullUser().GetId() != id || opened.GetFullUser().GetSettings() == nil || opened.GetFullUser().GetNotifySettings() == nil {
			t.Fatalf("getFullUser %s id %d settings %v notify %v", name, opened.GetFullUser().GetId(), opened.GetFullUser().GetSettings() != nil, opened.GetFullUser().GetNotifySettings() != nil)
		}
		var listed bool
		for _, u := range opened.GetUsers() {
			if u.GetId() == id && u.GetBot() && u.GetAccessHash().GetValue() != 0 {
				listed = true
			}
		}
		if !listed {
			t.Fatalf("getFullUser %s users did not include the bot", name)
		}
	}

	_, err = c.Invoke(ctx, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_725afbbc,
		Username:    "zzznone361",
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "USERNAME_NOT_OCCUPIED" {
		t.Fatalf("missing name: %v", err)
	}
}
