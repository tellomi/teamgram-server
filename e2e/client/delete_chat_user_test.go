package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestBasicGroupKickWritesDeleteUserServiceMessage is the official-client
// path: messages.deleteChatUser must put messageActionChatDeleteUser in the
// group while the removed user can still receive it.
func TestBasicGroupKickWritesDeleteUserServiceMessage(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "kick_a", "KickA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "kick_b", "KickB")
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()

	chatID, err := a.CreateChat(ctx, "kick-service", inputUser(bSelf.ID, bSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	up, err := a.Invoke(ctx, &mtproto.TLMessagesDeleteChatUser{
		Constructor: mtproto.CRC32_messages_deleteChatUser,
		ChatId:      chatID,
		UserId:      inputUser(bSelf.ID, bSelf.AccessHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updatesHaveDeleteUser(up, bSelf.ID) && !chatHistoryHasAction(t, a, chatID, mtproto.Predicate_messageActionChatDeleteUser) {
		t.Fatalf("no delete-user service message in %T", up)
	}
	if !chatHistoryHasAction(t, b, chatID, mtproto.Predicate_messageActionChatDeleteUser) {
		t.Fatal("removed user cannot see messageActionChatDeleteUser")
	}
	_ = aSelf
}

func chatHistoryHasAction(t *testing.T, c *Client, chatID int64, name string) bool {
	t.Helper()
	ctx, cancel := contextTimeout(20 * time.Second)
	defer cancel()
	page := historyPage(t, c, ctx, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        inputPeerChat(chatID),
		Limit:       20,
	})
	for _, m := range page {
		if m.GetAction().GetPredicateName() == name {
			return true
		}
	}
	return false
}

func updatesHaveDeleteUser(obj mtproto.TLObject, userID int64) bool {
	up, ok := obj.(*mtproto.Updates)
	if !ok {
		if box, ok := obj.(*mtproto.TLUpdates); ok {
			up = box.To_Updates()
		}
	}
	if up == nil {
		return false
	}
	for _, u := range up.GetUpdates() {
		msg := u.GetMessage_MESSAGE()
		if msg.GetAction().GetPredicateName() == mtproto.Predicate_messageActionChatDeleteUser && msg.GetAction().GetUserId() == userID {
			return true
		}
	}
	return false
}
