package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestNativeProtocolGaps covers business-bot errors, history import, quote
// text, and the chat-theme service message both peers can read back.
func TestNativeProtocolGaps(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "gap_a", "GapA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "gap_b", "GapB")
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()

	bots, err := a.Invoke(ctx, &mtproto.TLAccountGetConnectedBots{
		Constructor: mtproto.CRC32_account_getConnectedBots,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bots.(*mtproto.TLAccountConnectedBots); !ok {
		t.Fatalf("getConnectedBots %T", bots)
	}

	_, err = a.Invoke(ctx, &mtproto.TLAccountToggleConnectedBotPaused{
		Constructor: mtproto.CRC32_account_toggleConnectedBotPaused,
		Peer:        inputPeerUser(bSelf.ID, bSelf.AccessHash),
		Paused:      mtproto.ToBool(true),
	})
	if err == nil || !strings.Contains(err.Error(), "BOT_NOT_CONNECTED_YET") {
		t.Fatalf("toggle with no bot: %v", err)
	}

	_, err = a.Invoke(ctx, &mtproto.TLAccountUpdateConnectedBot{
		Constructor:                           mtproto.CRC32_account_updateConnectedBot_66a08c7e,
		Bot:                                   inputUser(bSelf.ID, bSelf.AccessHash),
		Recipients_INPUTBUSINESSBOTRECIPIENTS: mtproto.MakeTLInputBusinessBotRecipients(&mtproto.InputBusinessBotRecipients{Contacts: true}).To_InputBusinessBotRecipients(),
	})
	if err == nil || !strings.Contains(err.Error(), "BOT_BUSINESS_MISSING") {
		t.Fatalf("non-business bot: %v", err)
	}

	_, err = a.Invoke(ctx, &mtproto.TLMessagesInitHistoryImport{
		Constructor: mtproto.CRC32_messages_initHistoryImport,
		Peer:        inputPeerUser(bSelf.ID, bSelf.AccessHash),
		File:        mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 4, Parts: 1, Name: "chat.json"}).To_InputFile(),
	})
	if err == nil || !strings.Contains(err.Error(), "IMPORT_NOT_AVAILABLE") {
		t.Fatalf("init import: %v", err)
	}

	id, err := a.SendText(ctx, inputPeerUser(bSelf.ID, bSelf.AccessHash), "buy milk today", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerUser(bSelf.ID, bSelf.AccessHash),
		Message:     "nope",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: id,
			QuoteText:    wrapperspb.String("bread"),
		}).To_InputReplyTo(),
	})
	if err == nil || !strings.Contains(err.Error(), "QUOTE_TEXT_INVALID") {
		t.Fatalf("bad quote: %v", err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        inputPeerUser(bSelf.ID, bSelf.AccessHash),
		Message:     "yes",
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: id,
			QuoteText:    wrapperspb.String("milk"),
			QuoteOffset:  wrapperspb.Int32(4),
			TodoItemId:   wrapperspb.Int32(1),
		}).To_InputReplyTo(),
	}); err != nil {
		t.Fatalf("good quote: %v", err)
	}

	if _, err = a.Invoke(ctx, &mtproto.TLMessagesSetChatTheme{
		Constructor: mtproto.CRC32_messages_setChatTheme_81202c9,
		Peer:        inputPeerUser(bSelf.ID, bSelf.AccessHash),
		Theme:       mtproto.MakeTLInputChatTheme(&mtproto.InputChatTheme{Emoticon: "🎄"}).To_InputChatTheme(),
	}); err != nil {
		t.Fatal(err)
	}
	if !historyHasAction(t, a, inputPeerUser(bSelf.ID, bSelf.AccessHash), mtproto.Predicate_messageActionSetChatTheme) {
		t.Fatal("caller history has no setChatTheme service message")
	}
	if !historyHasAction(t, b, inputPeerUser(aSelf.ID, aSelf.AccessHash), mtproto.Predicate_messageActionSetChatTheme) {
		t.Fatal("peer history has no setChatTheme service message")
	}
}

func historyHasAction(t *testing.T, c *Client, peer *mtproto.InputPeer, name string) bool {
	t.Helper()
	ctx, cancel := contextTimeout(20 * time.Second)
	defer cancel()
	page := historyPage(t, c, ctx, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        peer,
		Limit:       20,
	})
	for _, m := range page {
		if m.GetAction().GetPredicateName() == name {
			return true
		}
	}
	return false
}
