package e2e

import (
	"fmt"
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
	// setChatTheme returns after Kafka accepts both inbox records. getHistory
	// reads the messages rows, and the inbox consumer writes those after the
	// RPC returns. One read can miss a row that is about to land. The action
	// still has to show up for both peers.
	waitHistoryAction(t, a, inputPeerUser(bSelf.ID, bSelf.AccessHash), mtproto.Predicate_messageActionSetChatTheme, "caller history has no setChatTheme service message")
	waitHistoryAction(t, b, inputPeerUser(aSelf.ID, aSelf.AccessHash), mtproto.Predicate_messageActionSetChatTheme, "peer history has no setChatTheme service message")
}

func waitHistoryAction(t *testing.T, c *Client, peer *mtproto.InputPeer, name, fail string) {
	t.Helper()
	// The sender's copy is written by the inbox consumer after sendMessage
	// returns. One getHistory can run before that row exists. A single RPC
	// error during the wait is not the failure; the action still has to appear.
	ctx, cancel := contextTimeout(10 * time.Second)
	defer cancel()
	err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		res, err := c.Invoke(ctx, &mtproto.TLMessagesGetHistory{
			Constructor: mtproto.CRC32_messages_getHistory,
			Peer:        peer,
			Limit:       20,
		})
		if err != nil {
			if ctx.Err() != nil {
				return false, nil
			}
			return false, err
		}
		msgs, ok := messagesOf(res)
		if !ok {
			return false, fmt.Errorf("getHistory %T", res)
		}
		for _, m := range msgs {
			if m.GetAction().GetPredicateName() == name {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("%s: %v", fail, err)
	}
}
