package richmsg

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func markdown(text string) *mtproto.InputRichMessage {
	return mtproto.MakeTLInputRichMessageMarkdown(&mtproto.InputRichMessage{
		Markdown: text,
	}).To_InputRichMessage()
}

func paragraphInput(text string) *mtproto.InputRichMessage {
	plain := mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: text}).To_RichText()
	block := mtproto.MakeTLPageBlockParagraph(&mtproto.PageBlock{Text: plain}).To_PageBlock()
	return mtproto.MakeTLInputRichMessage(&mtproto.InputRichMessage{
		Blocks: []*mtproto.PageBlock{block},
	}).To_InputRichMessage()
}

func mustEncode(t *testing.T, obj interface {
	Encode(*mtproto.EncodeBuf, int32) error
}) {
	t.Helper()
	buf := mtproto.NewEncodeBuf(4096)
	if err := obj.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	if buf.GetBuf() == nil || len(buf.GetBuf()) == 0 {
		t.Fatal("empty encode")
	}
}

func TestTranslateRichTextAndBlocks(t *testing.T) {
	if _, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "nope",
		Text:   []*mtproto.InputRichMessage{markdown("hello")},
	}, nil); status.Convert(err).Message() != "TO_LANG_INVALID" {
		t.Fatal(err)
	}
	long := ""
	for i := 0; i < 65; i++ {
		long += "a"
	}
	if _, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "zh",
		Tone:   &wrapperspb.StringValue{Value: long},
		Text:   []*mtproto.InputRichMessage{markdown("hello")},
	}, nil); status.Convert(err).Message() != "TO_LANG_INVALID" {
		t.Fatal(err)
	}
	out, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "zh-hans",
		Text:   []*mtproto.InputRichMessage{markdown("**hello**"), paragraphInput("hello")},
	}, nil)
	if err != nil || len(out.GetResult()) != 2 || visible(out.GetResult()[0]) != "你好" || visible(out.GetResult()[1]) != "你好" {
		t.Fatalf("translate %v %#v", err, out)
	}
	mustEncode(t, out)
	if out.GetResult()[0].GetPhotos() == nil || out.GetResult()[0].GetDocuments() == nil {
		t.Fatal("vectors")
	}

	if _, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{ToLang: "en"}, nil); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatal(err)
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 4}).To_InputPeer()
	if _, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "en", Peer: peer,
	}, nil); status.Convert(err).Message() != "MESSAGE_IDS_EMPTY" {
		t.Fatal(err)
	}
	if _, err := Translate(context.Background(), 1, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "en", Peer: peer, Id: []int32{0},
	}, nil); status.Convert(err).Message() != "MESSAGE_ID_INVALID" {
		t.Fatal(err)
	}

	chatbox.ResetShared()
	p := mtproto.MakePeerUtil(mtproto.PEER_USER, 4)
	chatbox.Shared().RememberPlain(context.Background(), 3, p, 11, "hello", nil)
	byID, err := Translate(context.Background(), 3, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "zh", Peer: peer, Id: []int32{11},
	}, nil)
	if err != nil || visible(byID.GetResult()[0]) != "你好" {
		t.Fatalf("id %v %#v", err, byID)
	}
	loaded, err := Translate(context.Background(), 9, &mtproto.TLMessagesTranslateRichMessage{
		ToLang: "en", Peer: peer, Id: []int32{8},
	}, func(context.Context, int64, *mtproto.PeerUtil, int32) (string, []*mtproto.MessageEntity, bool) {
		return "thanks", nil, true
	})
	if err != nil || visible(loaded.GetResult()[0]) != "thanks" {
		t.Fatalf("load %v %#v", err, loaded)
	}
}

func TestComposeRichKeepsSourceWithoutModel(t *testing.T) {
	if _, err := Compose(nil); status.Convert(err).Message() != "MESSAGE_EMPTY" {
		t.Fatal(err)
	}
	if _, err := Compose(&mtproto.TLMessagesComposeRichMessageWithAI{}); status.Convert(err).Message() != "MESSAGE_EMPTY" {
		t.Fatal(err)
	}
	text := markdown("hello there")
	if _, err := Compose(&mtproto.TLMessagesComposeRichMessageWithAI{Text: text}); status.Convert(err).Message() != "AI_COMPOSE_TASK_MISSING" {
		t.Fatal(err)
	}
	if _, err := Compose(&mtproto.TLMessagesComposeRichMessageWithAI{
		Text: text,
		Tone: mtproto.MakeTLInputAiComposeToneDefault(&mtproto.InputAiComposeTone{Tone: "not-a-tone"}).To_InputAiComposeTone(),
	}); status.Convert(err).Message() != "AICOMPOSE_TONE_INVALID" {
		t.Fatal(err)
	}
	kept, err := Compose(&mtproto.TLMessagesComposeRichMessageWithAI{
		Proofread: true,
		Text:      text,
		Tone:      mtproto.MakeTLInputAiComposeToneDefault(&mtproto.InputAiComposeTone{Tone: "friendly"}).To_InputAiComposeTone(),
	})
	if err != nil || visible(kept.GetResult()) != "hello there" {
		t.Fatalf("proofread %v %#v", err, kept)
	}
	mustEncode(t, kept)
	zh, err := Compose(&mtproto.TLMessagesComposeRichMessageWithAI{
		Text:            markdown("hello"),
		TranslateToLang: &wrapperspb.StringValue{Value: "zh-hans"},
	})
	if err != nil || visible(zh.GetResult()) != "你好" {
		t.Fatalf("compose translate %v %#v", err, zh)
	}
	if _, err := Compose(&mtproto.TLMessagesComposeRichMessageWithAI{
		Proofread:       true,
		Text:            text,
		TranslateToLang: &wrapperspb.StringValue{Value: "nope"},
	}); status.Convert(err).Message() != "TO_LANG_INVALID" {
		t.Fatal(err)
	}
}
