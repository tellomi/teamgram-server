package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestBasicGroupHistoryTTL is rows 200 and 201.
// messages.getFullChat.ttl_period follows the chat timer, and a normal
// member cannot change it.
func TestBasicGroupHistoryTTL(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610700", "TtlOwner")
	member, memberSelf := dialPhone(t, ctx, cfg, "9996610701", "TtlMember")

	chatID, err := owner.CreateChat(ctx, "ttl room", inputUser(memberSelf.ID, memberSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChat(chatID)
	_, err = member.Invoke(ctx, &mtproto.TLMessagesSetHistoryTTL{
		Constructor: mtproto.CRC32_messages_setHistoryTTL,
		Peer:        peer,
		Period:      86400,
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member setHistoryTTL: %v", err)
	}
	if got := fullChatTTL(t, member, chatID); got != 0 {
		t.Fatalf("member write changed ttl_period to %d", got)
	}

	if err = setHistoryTTL(ctx, owner, peer, 86400); err != nil {
		t.Fatal(err)
	}
	if got := fullChatTTL(t, owner, chatID); got != 86400 {
		t.Fatalf("owner ttl_period = %d", got)
	}
	if got := fullChatTTL(t, member, chatID); got != 86400 {
		t.Fatalf("member ttl_period = %d", got)
	}
}

func fullChatTTL(t *testing.T, c *Client, chatID int64) int32 {
	t.Helper()
	ctx, cancel := contextTimeout(20 * time.Second)
	defer cancel()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetFullChat{
		Constructor: mtproto.CRC32_messages_getFullChat,
		ChatId:      chatID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return asChatFull(t, obj).GetTtlPeriod().GetValue()
}
