package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestRichTranslateAndCompose is the chat-screen path that used to return
// ENTERPRISE_IS_BLOCKED: translating a formatted draft, translating a sent
// message, and the AI editor button. There is no composer model, so
// proofread keeps the source text. translate_to_lang uses the same lexicon
// as messages.translateText.
func TestRichTranslateAndCompose(t *testing.T) {
	a, _, _, bSelf := sharedPair(t)
	ctx, cancel := contextTimeout(40 * time.Second)
	defer cancel()
	peer := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	hello := mtproto.MakeTLInputRichMessageMarkdown(&mtproto.InputRichMessage{Markdown: "hello"}).To_InputRichMessage()

	_, err := a.Invoke(ctx, &mtproto.TLMessagesTranslateRichMessage{
		Constructor: mtproto.CRC32_messages_translateRichMessage,
		ToLang:      "nope",
		Text:        []*mtproto.InputRichMessage{hello},
	})
	assertRichError(t, err, "TO_LANG_INVALID")

	translated, err := a.Invoke(ctx, &mtproto.TLMessagesTranslateRichMessage{
		Constructor: mtproto.CRC32_messages_translateRichMessage,
		ToLang:      "zh-hans",
		Text:        []*mtproto.InputRichMessage{hello},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := translatedRich(t, translated)
	if len(rows) != 1 || richPlain(rows[0]) != "你好" {
		t.Fatalf("draft translate %#v", rows)
	}

	_, err = a.Invoke(ctx, &mtproto.TLMessagesTranslateRichMessage{
		Constructor: mtproto.CRC32_messages_translateRichMessage,
		ToLang:      "en",
		Peer:        peer,
	})
	assertRichError(t, err, "MESSAGE_IDS_EMPTY")

	id, err := a.SendText(ctx, peer, "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	byID, err := a.Invoke(ctx, &mtproto.TLMessagesTranslateRichMessage{
		Constructor: mtproto.CRC32_messages_translateRichMessage,
		ToLang:      "zh",
		Peer:        peer,
		Id:          []int32{id},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows = translatedRich(t, byID)
	if len(rows) != 1 || richPlain(rows[0]) != "你好" {
		t.Fatalf("message translate %#v", rows)
	}

	_, err = a.Invoke(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Constructor: mtproto.CRC32_messages_composeRichMessageWithAI,
	})
	assertRichError(t, err, "MESSAGE_EMPTY")

	_, err = a.Invoke(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Constructor: mtproto.CRC32_messages_composeRichMessageWithAI,
		Text:        hello,
	})
	assertRichError(t, err, "AI_COMPOSE_TASK_MISSING")

	kept, err := a.Invoke(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Constructor: mtproto.CRC32_messages_composeRichMessageWithAI,
		Proofread:   true,
		Text:        hello,
	})
	if err != nil {
		t.Fatal(err)
	}
	if richPlain(composedRich(t, kept)) != "hello" {
		t.Fatalf("proofread %#v", kept)
	}

	composed, err := a.Invoke(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Constructor:     mtproto.CRC32_messages_composeRichMessageWithAI,
		Text:            hello,
		TranslateToLang: &wrapperspb.StringValue{Value: "zh-hans"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if richPlain(composedRich(t, composed)) != "你好" {
		t.Fatalf("compose translate %#v", composed)
	}
}

// TestConfirmBotConnection is the dialog a client shows when a user connects
// a bot. A person and an empty id are BOT_ID_INVALID. A bot created with
// bots.createBot confirms and can be confirmed again.
func TestConfirmBotConnection(t *testing.T) {
	a, _, _, bSelf := sharedPair(t)
	ctx, cancel := contextTimeout(40 * time.Second)
	defer cancel()

	_, err := a.Invoke(ctx, &mtproto.TLAccountConfirmBotConnection{
		Constructor: mtproto.CRC32_account_confirmBotConnection,
		BotId:       mtproto.MakeTLInputUserEmpty(nil).To_InputUser(),
	})
	assertRichError(t, err, "BOT_ID_INVALID")

	_, err = a.Invoke(ctx, &mtproto.TLAccountConfirmBotConnection{
		Constructor: mtproto.CRC32_account_confirmBotConnection,
		BotId:       mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
	})
	assertRichError(t, err, "BOT_ID_INVALID")

	_, err = a.Invoke(ctx, &mtproto.TLAccountConfirmBotConnection{
		Constructor: mtproto.CRC32_account_confirmBotConnection,
		BotId:       inputUser(bSelf.ID, bSelf.AccessHash),
	})
	assertRichError(t, err, "BOT_ID_INVALID")

	// user.BotFatherId. The manager does not have to be a bot this user owns.
	const botFatherID int64 = 6
	var created *mtproto.TLUser
	var last error
	for i := 0; i < 4; i++ {
		obj, err := a.Invoke(ctx, &mtproto.TLBotsCreateBot{
			Constructor: mtproto.CRC32_bots_createBot,
			Name:        "E2E Helper",
			Username:    randomUsername() + "bot",
			ManagerId:   inputUser(botFatherID, 0),
		})
		if err != nil {
			last = err
			if rpc, ok := asRPCError(err); ok && rpc.Message == "USERNAME_OCCUPIED" {
				continue
			}
			t.Fatal(err)
		}
		user, ok := obj.(*mtproto.TLUser)
		if !ok || user.GetId() == 0 || user.GetAccessHash().GetValue() == 0 {
			t.Fatalf("createBot %T %#v", obj, obj)
		}
		created = user
		break
	}
	if created == nil {
		t.Fatal(last)
	}

	ok, err := a.Invoke(ctx, &mtproto.TLAccountConfirmBotConnection{
		Constructor: mtproto.CRC32_account_confirmBotConnection,
		BotId:       inputUser(created.GetId(), created.GetAccessHash().GetValue()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, yes := ok.(*mtproto.TLBoolTrue); !yes {
		t.Fatalf("confirm %T", ok)
	}
	again, err := a.Invoke(ctx, &mtproto.TLAccountConfirmBotConnection{
		Constructor: mtproto.CRC32_account_confirmBotConnection,
		BotId:       inputUser(created.GetId(), created.GetAccessHash().GetValue()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, yes := again.(*mtproto.TLBoolTrue); !yes {
		t.Fatalf("confirm again %T", again)
	}

	_, err = a.Invoke(ctx, &mtproto.TLAccountConfirmBotConnection{
		Constructor: mtproto.CRC32_account_confirmBotConnection,
		BotId:       inputUser(created.GetId(), created.GetAccessHash().GetValue()+1),
	})
	assertRichError(t, err, "BOT_ID_INVALID")
}

// TestRichRequestsEncode checks the requests above round-trip before a server
// is required. A flag without a constructor makes the session drop the RPC.
func TestRichRequestsEncode(t *testing.T) {
	hello := mtproto.MakeTLInputRichMessageMarkdown(&mtproto.InputRichMessage{Markdown: "hello"}).To_InputRichMessage()
	reqs := []mtproto.TLObject{
		&mtproto.TLMessagesTranslateRichMessage{
			Constructor: mtproto.CRC32_messages_translateRichMessage,
			ToLang:      "zh-hans",
			Text:        []*mtproto.InputRichMessage{hello},
		},
		&mtproto.TLMessagesTranslateRichMessage{
			Constructor: mtproto.CRC32_messages_translateRichMessage,
			ToLang:      "zh",
			Peer:        inputPeerUser(2, 3),
			Id:          []int32{11},
		},
		&mtproto.TLMessagesComposeRichMessageWithAI{
			Constructor:     mtproto.CRC32_messages_composeRichMessageWithAI,
			Proofread:       true,
			Text:            hello,
			TranslateToLang: &wrapperspb.StringValue{Value: "zh-hans"},
		},
		&mtproto.TLAccountConfirmBotConnection{
			Constructor: mtproto.CRC32_account_confirmBotConnection,
			BotId:       inputUser(9, 8),
		},
		&mtproto.TLBotsCreateBot{
			Constructor: mtproto.CRC32_bots_createBot,
			Name:        "E2E Helper",
			Username:    "qabbot",
			ManagerId:   inputUser(6, 0),
		},
	}
	for _, req := range reqs {
		raw, err := encodeTL(req)
		if err != nil {
			t.Fatalf("%T: %v", req, err)
		}
		if err := smokeDecodeExact(raw); err != nil {
			t.Fatalf("%T: %v", req, err)
		}
	}
}

func assertRichError(t *testing.T, err error, want string) {
	t.Helper()
	rpc, ok := asRPCError(err)
	if !ok {
		t.Fatalf("error %v, want %s", err, want)
	}
	msg := strings.ToUpper(rpc.Message)
	if strings.Contains(msg, "ENTERPRISE") || strings.Contains(msg, "NOT_IMPL") {
		t.Fatalf("stub error %s", rpc.Message)
	}
	if rpc.Message != want {
		t.Fatalf("got %s, want %s", rpc.Message, want)
	}
}

func translatedRich(t *testing.T, obj mtproto.TLObject) []*mtproto.RichMessage {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesTranslatedRichMessage:
		return v.GetResult()
	case *mtproto.Messages_TranslatedRichMessage:
		return v.GetResult()
	default:
		t.Fatalf("translated %T", obj)
		return nil
	}
}

func composedRich(t *testing.T, obj mtproto.TLObject) *mtproto.RichMessage {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesComposedRichMessageWithAI:
		return v.GetResult()
	case *mtproto.Messages_ComposedRichMessageWithAI:
		return v.GetResult()
	default:
		t.Fatalf("composed %T", obj)
		return nil
	}
}

func richPlain(msg *mtproto.RichMessage) string {
	if msg == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range msg.GetBlocks() {
		if block == nil || block.GetText() == nil {
			continue
		}
		b.WriteString(block.GetText().GetText_STRING())
	}
	return b.String()
}
