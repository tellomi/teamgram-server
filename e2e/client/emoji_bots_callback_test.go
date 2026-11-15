package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestEmojiStatusesAndBuiltinBots covers spreadsheet rows 213, 239, and 230.
// Phone 9996610717.
func TestEmojiStatusesAndBuiltinBots(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610717", "emoji")

	obj := invokeOK(t, a, &mtproto.TLAccountGetDefaultEmojiStatuses{
		Constructor: mtproto.CRC32_account_getDefaultEmojiStatuses,
	})
	list, ok := obj.(*mtproto.TLAccountEmojiStatuses)
	if !ok || len(list.GetStatuses()) == 0 {
		t.Fatalf("default statuses %T len %d", obj, len(list.GetStatuses()))
	}
	channel := invokeOK(t, a, &mtproto.TLAccountGetChannelDefaultEmojiStatuses{
		Constructor: mtproto.CRC32_account_getChannelDefaultEmojiStatuses,
	})
	chList, ok := channel.(*mtproto.TLAccountEmojiStatuses)
	if !ok || len(chList.GetStatuses()) == 0 {
		t.Fatalf("channel statuses %T", channel)
	}

	c, cancelResolve := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelResolve()
	resolved, err := a.Invoke(c, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_725afbbc,
		Username:    "gif",
	})
	if err != nil {
		t.Fatal(err)
	}
	peer, ok := resolved.(*mtproto.TLContactsResolvedPeer)
	if !ok {
		t.Fatalf("resolve %T", resolved)
	}
	var access int64
	for _, u := range peer.GetUsers() {
		if u.GetId() == mtproto.BotGifId {
			access = u.GetAccessHash().GetValue()
		}
	}
	bot := mtproto.MakeTLInputUser(&mtproto.InputUser{
		UserId:     mtproto.BotGifId,
		AccessHash: access,
	}).To_InputUser()
	invokeOK(t, a, &mtproto.TLMessagesStartBot{
		Constructor: mtproto.CRC32_messages_startBot,
		Bot:         bot,
		Peer:        inputPeerUser(mtproto.BotGifId, access),
		RandomId:    time.Now().UnixNano(),
	})
	msgID, err := a.SendText(ctx, inputPeerUser(mtproto.BotGifId, access), "hello gif", nil)
	if err != nil {
		t.Fatal(err)
	}
	hist, err := a.History(ctx, inputPeerUser(mtproto.BotGifId, access), 10)
	if err != nil {
		t.Fatal(err)
	}
	if idOfText(hist, "hello gif") == 0 || idOfText(hist, "/start") == 0 {
		t.Fatalf("gif history missing start or text, id %d", msgID)
	}
	_, err = a.Invoke(ctx, &mtproto.TLMessagesGetBotCallbackAnswer{
		Constructor: mtproto.CRC32_messages_getBotCallbackAnswer,
		Peer:        inputPeerUser(mtproto.BotGifId, access),
		MsgId:       msgID,
		Data:        []byte("go"),
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "DATA_INVALID" {
		t.Fatalf("callback on a plain message: %v", err)
	}
}
