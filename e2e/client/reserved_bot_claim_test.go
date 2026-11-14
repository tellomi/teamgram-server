package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestReservedBotNamesCannotBeClaimed is account and channel username writes
// for the built-in bots. A new claim is USERNAME_OCCUPIED. The channel title
// is not a reserved word.
func TestReservedBotNamesCannotBeClaimed(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileIOS, "bot_resolve", "BotResolve")
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()

	names := []string{"bing", "BotFather", "wiki", "imdb", "bold", "music", "sticker", "venue"}
	for _, name := range names {
		_, err := c.Invoke(ctx, &mtproto.TLAccountCheckUsername{
			Constructor: mtproto.CRC32_account_checkUsername,
			Username:    name,
		})
		rpc, ok := asRPCError(err)
		if !ok || rpc.Message != "USERNAME_OCCUPIED" {
			t.Fatalf("account.checkUsername(%s) = %v", name, err)
		}
		_, err = c.Invoke(ctx, &mtproto.TLAccountUpdateUsername{
			Constructor: mtproto.CRC32_account_updateUsername,
			Username:    name,
		})
		rpc, ok = asRPCError(err)
		if !ok || rpc.Message != "USERNAME_OCCUPIED" {
			t.Fatalf("account.updateUsername(%s) = %v", name, err)
		}
	}

	created, err := c.Invoke(ctx, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Megagroup:   true,
		Title:       "claim room",
	})
	if err != nil {
		t.Fatal(err)
	}
	var chID, chHash int64
	for _, chat := range chatsOf(created) {
		if chat != nil && chat.GetMegagroup() {
			chID = chat.GetId()
			chHash = chatAccessHash(chat)
		}
	}
	if chID == 0 {
		t.Fatalf("create channel %T", created)
	}
	channel := inputChannel(chID, chHash)
	for _, name := range []string{"bing", "BotFather"} {
		_, err = c.Invoke(ctx, &mtproto.TLChannelsCheckUsername{
			Constructor: mtproto.CRC32_channels_checkUsername,
			Channel:     channel,
			Username:    name,
		})
		rpc, ok := asRPCError(err)
		if !ok || rpc.Message != "USERNAME_OCCUPIED" {
			t.Fatalf("channels.checkUsername(%s) = %v", name, err)
		}
		_, err = c.Invoke(ctx, &mtproto.TLChannelsUpdateUsername{
			Constructor: mtproto.CRC32_channels_updateUsername,
			Channel:     channel,
			Username:    name,
		})
		rpc, ok = asRPCError(err)
		if !ok || rpc.Message != "USERNAME_OCCUPIED" {
			t.Fatalf("channels.updateUsername(%s) = %v", name, err)
		}
	}
	_, err = c.Invoke(ctx, &mtproto.TLChannelsToggleUsername{
		Constructor: mtproto.CRC32_channels_toggleUsername,
		Channel:     channel,
		Username:    "bing",
		Active:      mtproto.BoolTrue,
	})
	rpc, ok := asRPCError(err)
	if !ok || (rpc.Message != "USERNAME_OCCUPIED" && rpc.Message != "USERNAME_INVALID") {
		t.Fatalf("channels.toggleUsername(bing) = %v", err)
	}
}
