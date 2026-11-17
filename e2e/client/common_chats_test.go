package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestCommonChatsIncludeMegagroup checks messages.getCommonChats returns a
// shared basic group and a shared supergroup, and not a user outside them.
// Phones 9996610763–9996610765, code 11111, layer 232.
func TestCommonChatsIncludeMegagroup(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610763", "CommonA")
	b, bSelf := dialPhone(t, ctx, cfg, "9996610764", "CommonB")
	_, cSelf := dialPhone(t, ctx, cfg, "9996610765", "CommonC")

	basicID, err := a.CreateChat(ctx, "LaneCommon", inputUser(bSelf.ID, bSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	megaID, megaHash, err := a.CreateMegagroup(ctx, "LaneMega")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.InviteToChannel(ctx, inputChannel(megaID, megaHash), inputUser(bSelf.ID, bSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	got := commonIDs(t, a, inputUser(bSelf.ID, bSelf.AccessHash))
	if !got[basicID] || !got[megaID] {
		t.Fatalf("A sees B: basic=%v mega=%v ids=%v", got[basicID], got[megaID], got)
	}
	back := commonIDs(t, b, inputUser(aSelf.ID, aSelf.AccessHash))
	if !back[basicID] || !back[megaID] {
		t.Fatalf("B sees A: basic=%v mega=%v ids=%v", back[basicID], back[megaID], back)
	}
	outsider := commonIDs(t, a, inputUser(cSelf.ID, cSelf.AccessHash))
	if outsider[basicID] || outsider[megaID] {
		t.Fatalf("C is not in the groups: %v", outsider)
	}
}

func commonIDs(t *testing.T, c *Client, user *mtproto.InputUser) map[int64]bool {
	t.Helper()
	obj := invokeOK(t, c, &mtproto.TLMessagesGetCommonChats{
		Constructor: mtproto.CRC32_messages_getCommonChats,
		UserId:      user,
		Limit:       100,
	})
	var chats []*mtproto.Chat
	switch v := obj.(type) {
	case *mtproto.TLMessagesChats:
		chats = v.GetChats()
	case *mtproto.TLMessagesChatsSlice:
		chats = v.GetChats()
	case *mtproto.Messages_Chats:
		chats = v.GetChats()
	default:
		t.Fatalf("messages.getCommonChats returned %T", obj)
	}
	ids := map[int64]bool{}
	for _, chat := range chats {
		if chat != nil {
			ids[chat.GetId()] = true
		}
	}
	return ids
}
