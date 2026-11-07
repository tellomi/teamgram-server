package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestRichTranslateAndCompose is the chat-screen path that used to return
// ENTERPRISE_IS_BLOCKED. Translation uses the self-hosted sidecar only.
// CI does not set TELLOMI_TRANSLATE_URL, so a Chinese request is
// TRANSLATE_REQ_FAILED. Compose still uses Bailian when TELLOMI_AI_COMPOSE
// is on; CI leaves that off, so a valid compose is AICOMPOSE_TIMEOUT.
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

	_, err = a.Invoke(ctx, &mtproto.TLMessagesTranslateRichMessage{
		Constructor: mtproto.CRC32_messages_translateRichMessage,
		ToLang:      "zh-hans",
		Text:        []*mtproto.InputRichMessage{hello},
	})
	assertRichError(t, err, "TRANSLATE_REQ_FAILED")

	// peer and id share flag bit 0. An omitted id is not encoded, and the
	// session drops the frame. An empty vector is the call with no message ids.
	// to_lang is checked first, so this has to be Chinese to reach the id check.
	_, err = a.Invoke(ctx, &mtproto.TLMessagesTranslateRichMessage{
		Constructor: mtproto.CRC32_messages_translateRichMessage,
		ToLang:      "zh",
		Peer:        peer,
		Id:          []int32{},
	})
	assertRichError(t, err, "MESSAGE_IDS_EMPTY")

	id, err := a.SendText(ctx, peer, "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Invoke(ctx, &mtproto.TLMessagesTranslateRichMessage{
		Constructor: mtproto.CRC32_messages_translateRichMessage,
		ToLang:      "zh",
		Peer:        peer,
		Id:          []int32{id},
	})
	assertRichError(t, err, "TRANSLATE_REQ_FAILED")

	_, err = a.Invoke(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Constructor: mtproto.CRC32_messages_composeRichMessageWithAI,
	})
	assertRichError(t, err, "MESSAGE_EMPTY")

	_, err = a.Invoke(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Constructor: mtproto.CRC32_messages_composeRichMessageWithAI,
		Text:        hello,
	})
	assertRichError(t, err, "AI_COMPOSE_TASK_MISSING")

	_, err = a.Invoke(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Constructor: mtproto.CRC32_messages_composeRichMessageWithAI,
		Proofread:   true,
		Text:        hello,
	})
	assertRichError(t, err, "AICOMPOSE_TIMEOUT")

	_, err = a.Invoke(ctx, &mtproto.TLMessagesComposeRichMessageWithAI{
		Constructor:     mtproto.CRC32_messages_composeRichMessageWithAI,
		Text:            hello,
		TranslateToLang: &wrapperspb.StringValue{Value: "zh-hans"},
	})
	assertRichError(t, err, "AICOMPOSE_TIMEOUT")
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
		&mtproto.TLMessagesTranslateRichMessage{
			Constructor: mtproto.CRC32_messages_translateRichMessage,
			ToLang:      "en",
			Peer:        inputPeerUser(2, 3),
			Id:          []int32{},
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
